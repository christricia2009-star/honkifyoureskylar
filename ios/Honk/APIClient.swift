import Foundation

enum APIError: LocalizedError {
    case message(String)
    case unauthorized

    var errorDescription: String? {
        switch self {
        case .message(let text): return text
        case .unauthorized: return "The horn does not recognize this session."
        }
    }
}

struct APIClient {
    var baseURL: URL
    var token: String?

    static var configuredBase: URL {
        if let raw = Bundle.main.object(forInfoDictionaryKey: "HONKAPIBaseURL") as? String,
           let url = URL(string: raw) {
            return url
        }
        return URL(string: "http://127.0.0.1:8080")!
    }

    func url(_ path: String) -> URL {
        var root = baseURL.absoluteString
        if root.hasSuffix("/") { root.removeLast() }
        let suffix = path.hasPrefix("/") ? path : "/" + path
        return URL(string: root + suffix) ?? baseURL
    }

    func get<T: Decodable>(_ path: String) async throws -> T {
        try await send(path, method: "GET", body: nil, expecting: T.self)
    }

    func post<T: Decodable>(_ path: String, json: Any? = nil) async throws -> T {
        try await send(path, method: "POST", body: json, expecting: T.self)
    }

    func put<T: Decodable>(_ path: String, json: Any) async throws -> T {
        try await send(path, method: "PUT", body: json, expecting: T.self)
    }

    func data(_ path: String, method: String, json: Any? = nil) async throws -> (Int, Data) {
        var request = URLRequest(url: url(path))
        request.httpMethod = method
        request.timeoutInterval = 40
        if let token, !token.isEmpty {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        if let json {
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try JSONSerialization.data(withJSONObject: json)
        }
        let (payload, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw APIError.message("The horn's server did not answer.")
        }
        if http.statusCode == 401 {
            throw APIError.unauthorized
        }
        return (http.statusCode, payload)
    }

    private func send<T: Decodable>(_ path: String, method: String, body: Any?, expecting: T.Type) async throws -> T {
        let (status, payload) = try await data(path, method: method, json: body)
        if !(200..<300).contains(status) {
            throw APIError.message(serverMessage(payload) ?? "The server answered \(status).")
        }
        do {
            return try JSONDecoder().decode(T.self, from: payload)
        } catch {
            throw APIError.message("The horn could not read the server's answer.")
        }
    }

    func serverMessage(_ payload: Data) -> String? {
        guard let object = try? JSONSerialization.jsonObject(with: payload) as? [String: Any] else { return nil }
        if let message = object["message"] as? String, !message.isEmpty { return message }
        if let error = object["error"] as? String, !error.isEmpty { return error }
        return nil
    }
}
