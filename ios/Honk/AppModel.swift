import AuthenticationServices
import Foundation
import UIKit
import UserNotifications

extension Notification.Name {
    static let honkDeviceToken = Notification.Name("honkDeviceToken")
}

enum PendingConfirm {
    case peek(String)
    case telemetry(String)

    var message: String {
        switch self {
        case .peek(let text), .telemetry(let text): return text
        }
    }
}

@MainActor
@Observable
final class AppModel: ASWebAuthenticationPresentationContextProviding {
    var token: String?
    var health: Health?
    var garage: Garage?
    var trips: [Trip] = []
    var drivers = DriversPage(drivers: [], problem: "", note: "")
    var alerts: [HonkAlert] = []
    var settings: Settings?
    var setup: SetupPage?
    var banner: String?
    var busy = false
    var pending: PendingConfirm?
    private var primedAlerts = false
    private var seenAlerts = Set<String>()
    private var pushObserver: NSObjectProtocol?
    private var streamTask: Task<Void, Never>?
    private var pollTask: Task<Void, Never>?
    private var authSession: ASWebAuthenticationSession?
    private let presenter = UIWindow()

    var signedIn: Bool { token != nil }
    private var client: APIClient {
        APIClient(baseURL: APIClient.configuredBase, token: token)
    }

    func bootstrap() async {
        token = Keychain.load()
        await reloadHealth()
        if pushObserver == nil {
            pushObserver = NotificationCenter.default.addObserver(forName: .honkDeviceToken, object: nil, queue: .main) { note in
                guard let hex = note.object as? String else { return }
                Task { @MainActor in await self.registerPush(hex) }
            }
        }
        guard token != nil else { return }
        await refreshAll()
        listen()
        requestNotifications()
    }

    func reloadHealth() async {
        do {
            health = try await APIClient(baseURL: APIClient.configuredBase, token: nil).get("/api/health")
        } catch {
            banner = error.localizedDescription
        }
    }

    func startDemo() async {
        await beginSession(path: "/api/session/demo", body: nil)
    }

    func signIn() {
        let url = client.url("/auth/tesla/start")
        let session = ASWebAuthenticationSession(url: url, callbackURLScheme: "honkifyoureskylar") { [weak self] callback, error in
            Task { @MainActor in
                guard let self else { return }
                if let error {
                    let ns = error as NSError
                    if ns.domain == ASWebAuthenticationSessionErrorDomain && ns.code == ASWebAuthenticationSessionError.canceledLogin.rawValue {
                        return
                    }
                    self.banner = error.localizedDescription
                    return
                }
                guard let callback else { return }
                let items = URLComponents(url: callback, resolvingAgainstBaseURL: false)?.queryItems ?? []
                if let problem = items.first(where: { $0.name == "error" })?.value {
                    self.banner = problem == "exchange" ? "Tesla did not finish sign-in." : "Sign-in expired. Try again."
                    return
                }
                guard let code = items.first(where: { $0.name == "code" })?.value else {
                    self.banner = "The horn did not get a sign-in code."
                    return
                }
                await self.exchange(code)
            }
        }
        session.presentationContextProvider = self
        session.prefersEphemeralWebBrowserSession = true
        authSession = session
        if !session.start() {
            banner = "The sign-in window did not open."
        }
    }

    func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor {
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        return scenes.flatMap(\.windows).first(where: \.isKeyWindow) ?? scenes.first?.windows.first ?? presenter
    }

    func exchange(_ code: String) async {
        await beginSession(path: "/auth/exchange", body: ["code": code])
    }

    func signOut() {
        streamTask?.cancel()
        pollTask?.cancel()
        Keychain.clear()
        token = nil
        garage = nil
        trips = []
        alerts = []
    }

    func refreshAll() async {
        await loadGarage()
        await loadTrips()
        await loadDrivers(refresh: false)
        await loadAlerts()
    }

    func loadGarage() async {
        do {
            garage = try await client.get("/api/garage")
            banner = nil
        } catch APIError.unauthorized {
            signOut()
        } catch {
            banner = error.localizedDescription
        }
    }

    func loadTrips() async {
        do {
            let page: TripsPage = try await client.get("/api/trips")
            trips = page.trips
        } catch APIError.unauthorized {
            signOut()
        } catch {
            banner = error.localizedDescription
        }
    }

    func loadDrivers(refresh: Bool) async {
        do {
            let path = refresh ? "/api/drivers?refresh=1" : "/api/drivers"
            drivers = try await client.get(path)
        } catch APIError.unauthorized {
            signOut()
        } catch {
            banner = error.localizedDescription
        }
    }

    func loadAlerts() async {
        do {
            let page: AlertsPage = try await client.get("/api/alerts")
            accept(page.alerts, announce: primedAlerts)
            primedAlerts = true
            settings = page.settings
        } catch APIError.unauthorized {
            signOut()
        } catch {
            banner = error.localizedDescription
        }
    }

    func loadSetup() async {
        do {
            setup = try await client.get("/api/setup")
        } catch {
            banner = error.localizedDescription
        }
    }

    func ack(_ id: String) async {
        do {
            let _: [String: Bool] = try await client.post("/api/setup/ack", json: ["id": id])
            await loadSetup()
        } catch {
            banner = error.localizedDescription
        }
    }

    func configureTelemetry() async {
        do {
            let (status, payload) = try await client.data("/api/setup/telemetry", method: "POST", json: ["confirm": false])
            if status == 409 {
                pending = .telemetry(client.serverMessage(payload) ?? "Sending the telemetry config is one signed command.")
                return
            }
            banner = client.serverMessage(payload) ?? "The config was not sent."
        } catch {
            banner = error.localizedDescription
        }
    }

    func confirmTelemetry() async {
        do {
            let (status, payload) = try await client.data("/api/setup/telemetry", method: "POST", json: ["confirm": true])
            if (200..<300).contains(status) {
                banner = "The car has the telemetry config."
                await loadSetup()
            } else {
                banner = client.serverMessage(payload) ?? "Tesla answered \(status)."
            }
        } catch {
            banner = error.localizedDescription
        }
    }

    func peek(confirm: Bool) async {
        busy = true
        defer { busy = false }
        do {
            let (status, payload) = try await client.data("/api/refresh", method: "POST", json: ["confirm": confirm, "loop": false])
            if status == 409 {
                pending = .peek(client.serverMessage(payload) ?? "This peeks once.")
                return
            }
            if !(200..<300).contains(status) {
                banner = client.serverMessage(payload) ?? "Tesla answered \(status)."
                return
            }
            garage = try JSONDecoder().decode(Garage.self, from: payload)
            banner = nil
        } catch APIError.unauthorized {
            signOut()
        } catch {
            banner = error.localizedDescription
        }
    }

    func save(settings next: Settings) async {
        do {
            let data = try JSONEncoder().encode(next)
            let object = try JSONSerialization.jsonObject(with: data)
            settings = try await client.put("/api/settings", json: object)
            banner = "The horn wrote that down."
        } catch {
            banner = error.localizedDescription
        }
    }

    func markRead(_ id: String) async {
        do {
            let _: [String: Bool] = try await client.post("/api/alerts/\(id)/read")
            if let index = alerts.firstIndex(where: { $0.id == id }) {
                var copy = alerts[index]
                copy = HonkAlert(id: copy.id, vin: copy.vin, kind: copy.kind, message: copy.message, createdAt: copy.createdAt, read: true)
                alerts[index] = copy
            }
        } catch {
            banner = error.localizedDescription
        }
    }

    func registerPush(_ hex: String) async {
        guard token != nil else { return }
        do {
            let _: [String: Bool] = try await client.post("/api/devices", json: ["pushToken": hex])
        } catch {
            banner = error.localizedDescription
        }
    }

    private func beginSession(path: String, body: Any?) async {
        busy = true
        defer { busy = false }
        do {
            let session: SessionResponse = try await APIClient(baseURL: APIClient.configuredBase, token: nil).post(path, json: body)
            Keychain.save(session.session)
            token = session.session
            seenAlerts = []
            await refreshAll()
            listen()
            requestNotifications()
        } catch {
            banner = error.localizedDescription
        }
    }

    private func listen() {
        streamTask?.cancel()
        pollTask?.cancel()
        guard let token else { return }
        let api = APIClient(baseURL: APIClient.configuredBase, token: token)
        streamTask = Task { [weak self] in
            var request = URLRequest(url: api.url("/api/events"))
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            request.timeoutInterval = 3600
            do {
                let (bytes, response) = try await URLSession.shared.bytes(for: request)
                guard let http = response as? HTTPURLResponse, http.statusCode == 200 else { throw APIError.message("stream") }
                var name = "message"
                var data = ""
                for try await line in bytes.lines {
                    if Task.isCancelled { return }
                    if line.isEmpty {
                        await self?.handle(event: name, json: data)
                        name = "message"
                        data = ""
                        continue
                    }
                    if line.hasPrefix(":") { continue }
                    if line.hasPrefix("event:") {
                        name = line.dropFirst(6).trimmingCharacters(in: .whitespaces)
                    } else if line.hasPrefix("data:") {
                        let piece = String(line.dropFirst(5)).trimmingCharacters(in: .whitespaces)
                        data = data.isEmpty ? piece : data + "\n" + piece
                    }
                }
            } catch {
                await self?.poll()
            }
        }
    }

    private func handle(event name: String, json: String) {
        guard let payload = json.data(using: .utf8), !json.isEmpty else { return }
        if name == "snapshot", let next = try? JSONDecoder().decode(Garage.self, from: payload) {
            garage = next
            return
        }
        if name == "alert", let alert = try? JSONDecoder().decode(HonkAlert.self, from: payload) {
            accept([alert], announce: true)
        }
    }

    func confirmPending() async {
        let choice = pending
        pending = nil
        switch choice {
        case .peek:
            await peek(confirm: true)
        case .telemetry:
            await confirmTelemetry()
        case nil:
            break
        }
    }

    private func accept(_ incoming: [HonkAlert], announce: Bool) {
        var merged = alerts
        for alert in incoming {
            if let index = merged.firstIndex(where: { $0.id == alert.id }) {
                merged[index] = alert
            } else {
                merged.insert(alert, at: 0)
            }
            let fresh = seenAlerts.insert(alert.id).inserted
            if announce && fresh && !alert.read {
                notify(alert.message)
            }
        }
        alerts = merged
    }

    private func poll() {
        pollTask?.cancel()
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(20))
                await self?.loadGarage()
                await self?.loadAlerts()
            }
        }
    }

    private func requestNotifications() {
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { _, _ in
            DispatchQueue.main.async {
                UIApplication.shared.registerForRemoteNotifications()
            }
        }
        UNUserNotificationCenter.current().delegate = NotificationBridge.shared
    }

    private func notify(_ message: String) {
        let content = UNMutableNotificationContent()
        content.title = "Honk if You're Skylar"
        content.body = message
        content.sound = .default
        UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil))
        UINotificationFeedbackGenerator().notificationOccurred(.warning)
    }
}

enum Keychain {
    private static let service = "com.example.honkifyoureskylar.session"

    static func save(_ token: String) {
        clear()
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecValueData as String: Data(token.utf8),
        ]
        SecItemAdd(query as CFDictionary, nil)
    }

    static func load() -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var item: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &item) == errSecSuccess,
              let data = item as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    static func clear() {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
        ]
        SecItemDelete(query as CFDictionary)
    }
}

final class NotificationBridge: NSObject, UNUserNotificationCenterDelegate {
    static let shared = NotificationBridge()

    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification) async -> UNNotificationPresentationOptions {
        [.banner, .sound]
    }
}

final class HonkAppDelegate: NSObject, UIApplicationDelegate {
    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
        let hex = deviceToken.map { String(format: "%02x", $0) }.joined()
        NotificationCenter.default.post(name: .honkDeviceToken, object: hex)
    }
}
