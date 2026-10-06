import CoreLocation
import CoreMotion
import Foundation

struct PhoneFix: Codable, Identifiable {
    var id: String
    var latitude: Double
    var longitude: Double
    var speedMph: Double
    var at: Date
    var activity: String
}

final class PhoneTrail: NSObject, CLLocationManagerDelegate, ObservableObject {
    static let shared = PhoneTrail()

    @Published private(set) var fixes: [PhoneFix] = []
    @Published private(set) var opens: [Date] = []
    @Published private(set) var sharing = false
    @Published private(set) var denied = false
    @Published private(set) var activity = "unknown"

    private let manager = CLLocationManager()
    private let motion = CMMotionActivityManager()
    private let motionQueue = OperationQueue()
    private var motionStarted = false
    private var driveActive = false
    private var loaded = false

    private override init() {
        super.init()
        motionQueue.maxConcurrentOperationCount = 1
        load()
    }

    func start() {
        manager.delegate = self
        manager.activityType = .otherNavigation
        apply(manager.authorizationStatus)
        startMotion()
    }

    func setDriveActive(_ active: Bool) {
        driveActive = active
        guard sharing else { return }
        manager.distanceFilter = active ? 15 : 40
        manager.desiredAccuracy = active ? kCLLocationAccuracyBest : kCLLocationAccuracyNearestTenMeters
        manager.startUpdatingLocation()
    }

    func noteOpened() {
        let now = Date()
        if let last = opens.last, now.timeIntervalSince(last) < 30 { return }
        opens.append(now)
        trim()
        save()
    }

    nonisolated func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let status = manager.authorizationStatus
        Task { @MainActor in self.apply(status) }
    }

    nonisolated func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        let rows: [(Double, Double, Double, Date)] = locations.compactMap { location in
            guard location.horizontalAccuracy >= 0, location.horizontalAccuracy <= 80 else { return nil }
            let mph = location.speed >= 0 ? location.speed * 2.2369362920544 : 0
            return (location.coordinate.latitude, location.coordinate.longitude, mph, location.timestamp)
        }
        guard !rows.isEmpty else { return }
        Task { @MainActor in self.record(rows) }
    }

    private func apply(_ status: CLAuthorizationStatus) {
        switch status {
        case .notDetermined:
            manager.requestWhenInUseAuthorization()
            sharing = false
        case .authorizedAlways:
            denied = false
            manager.allowsBackgroundLocationUpdates = true
            manager.pausesLocationUpdatesAutomatically = false
            manager.showsBackgroundLocationIndicator = true
            manager.distanceFilter = driveActive ? 15 : 40
            manager.desiredAccuracy = kCLLocationAccuracyNearestTenMeters
            manager.startUpdatingLocation()
            sharing = true
        case .authorizedWhenInUse:
            denied = false
            manager.requestAlwaysAuthorization()
            manager.distanceFilter = driveActive ? 15 : 40
            manager.desiredAccuracy = kCLLocationAccuracyNearestTenMeters
            manager.startUpdatingLocation()
            sharing = true
        case .denied, .restricted:
            denied = true
            sharing = false
            manager.stopUpdatingLocation()
        default:
            break
        }
    }

    private func startMotion() {
        guard !motionStarted, CMMotionActivityManager.isActivityAvailable() else { return }
        motionStarted = true
        motion.startActivityUpdates(to: motionQueue) { [weak self] activity in
            guard let activity else { return }
            let name = Self.activityName(activity)
            Task { @MainActor in self?.activity = name }
        }
    }

    private func record(_ rows: [(Double, Double, Double, Date)]) {
        for row in rows {
            let fix = PhoneFix(
                id: UUID().uuidString,
                latitude: row.0,
                longitude: row.1,
                speedMph: row.2,
                at: row.3,
                activity: activity
            )
            if let last = fixes.last, fix.at.timeIntervalSince(last.at) < 8,
               metersBetween(last.latitude, last.longitude, fix.latitude, fix.longitude) < 12 {
                continue
            }
            fixes.append(fix)
        }
        trim()
        save()
    }

    private func trim() {
        let cutoff = Date().addingTimeInterval(-8 * 24 * 60 * 60)
        fixes.removeAll { $0.at < cutoff }
        opens.removeAll { $0 < cutoff }
        if fixes.count > 4000 {
            fixes.removeFirst(fixes.count - 4000)
        }
    }

    private func load() {
        guard !loaded else { return }
        loaded = true
        guard let data = try? Data(contentsOf: Self.fileURL()) else { return }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        if let file = try? decoder.decode(TrailFile.self, from: data) {
            fixes = file.fixes
            opens = file.opens
        }
    }

    private func save() {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        guard let data = try? encoder.encode(TrailFile(fixes: fixes, opens: opens)) else { return }
        try? data.write(to: Self.fileURL(), options: .atomic)
    }

    private static func fileURL() -> URL {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        let dir = base.appendingPathComponent("Honk", isDirectory: true)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        return dir.appendingPathComponent("phone-trail.json")
    }

    private static func activityName(_ activity: CMMotionActivity) -> String {
        if activity.automotive { return "automotive" }
        if activity.cycling { return "cycling" }
        if activity.running { return "running" }
        if activity.walking { return "walking" }
        if activity.stationary { return "stationary" }
        return "unknown"
    }
}

private struct TrailFile: Codable {
    var fixes: [PhoneFix]
    var opens: [Date]
}
