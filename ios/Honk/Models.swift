import Foundation

struct Health: Decodable {
    var ok: Bool
    var demoAvailable: Bool
    var teslaConfigured: Bool
    var linked: Bool
    var domain: String
    var redirectUri: String
    var pairingUrl: String
    var publicKeyUrl: String
    var virtualKeyPaired: Bool
}

struct Horn: Decodable {
    var mood: String
    var line: String
}

struct Usage: Decodable {
    var month: String
    var demo: Bool
    var streamingSignals: Int
    var vehicleDataCalls: Int
    var wakes: Int
    var commands: Int
    var metadataCalls: Int
    var estimatedUsd: Double
    var monthlyCreditUsd: Double
    var remainingCreditUsd: Double
    var note: String
}

struct VehicleView: Decodable {
    var vin: String
    var name: String
    var state: String
    var lastSeen: String?
    var speedMph: Double?
    var gear: String
    var gearLabel: String
    var latitude: Double?
    var longitude: Double?
    var soc: Double?
    var estRangeMiles: Double?
    var chargeState: String
    var doorsOpen: Bool
    var doorSummary: String
    var locked: Bool?
    var driverSeatOccupied: Bool?
    var guestMode: Bool?
    var activeDriverNote: String
    var inTrip: Bool
}

struct LatLng: Decodable, Hashable {
    var latitude: Double
    var longitude: Double
    var speedMph: Double?
    var at: String?
}

struct Trip: Decodable, Identifiable {
    var id: String
    var vin: String
    var startedAt: String
    var endedAt: String?
    var maxSpeedMph: Double
    var distanceMiles: Double
    var overLimit: Bool
    var speedLimitMph: Double
    var polyline: [LatLng]
    var seatOccupied: Bool?
    var guestMode: Bool?
    var pendingClose: Bool
    var callout: String?
}

struct Garage: Decodable {
    var demo: Bool
    var linked: Bool
    var vehicle: VehicleView?
    var activeTrip: Trip?
    var horn: Horn
    var usage: Usage
    var serverTime: String
}

struct Driver: Decodable, Identifiable {
    var id: String { name + detail }
    var name: String
    var detail: String
    var sample: Bool
}

struct DriversPage: Decodable {
    var drivers: [Driver]
    var problem: String
    var note: String
}

struct HonkAlert: Decodable, Identifiable {
    var id: String
    var vin: String
    var kind: String
    var message: String
    var createdAt: String
    var read: Bool
}

struct Settings: Codable {
    var speedLimitMph: Double
    var curfewStart: String
    var curfewEnd: String
    var homeLatitude: Double
    var homeLongitude: Double
    var homeRadiusMeters: Double
    var homeSet: Bool
    var timezone: String
    var selectedVin: String
}

struct AlertsPage: Decodable {
    var alerts: [HonkAlert]
    var settings: Settings
}

struct TripsPage: Decodable {
    var trips: [Trip]
}

struct SetupItem: Decodable, Identifiable {
    var id: String
    var title: String
    var done: Bool
    var detail: String
}

struct SetupPage: Decodable {
    var items: [SetupItem]
    var pairingUrl: String
    var publicKeyUrl: String
    var redirectUri: String
    var scopes: [String]
    var locationNote: String
}

struct SessionResponse: Decodable {
    var session: String
    var demo: Bool
}

struct ConfirmBody: Decodable {
    var error: String
    var message: String?
    var willWake: Bool?
    var estimatedUsd: Double?
}

enum HonkFormat {
    static func when(_ raw: String?) -> String {
        guard let raw, let date = parse(raw) else { return "not yet" }
        return date.formatted(date: .abbreviated, time: .shortened)
    }

    static func clock(_ raw: String?) -> String {
        guard let raw, let date = parse(raw) else { return "" }
        return date.formatted(date: .omitted, time: .shortened)
    }

    static func mph(_ value: Double?) -> String {
        guard let value else { return "—" }
        return "\(Int(value.rounded())) mph"
    }

    static func miles(_ value: Double) -> String {
        String(format: "%.1f mi", value)
    }

    static func money(_ value: Double) -> String {
        value.formatted(.currency(code: "USD"))
    }

    private static func parse(_ raw: String) -> Date? {
        let fractional = ISO8601DateFormatter()
        fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = fractional.date(from: raw) { return date }
        let plain = ISO8601DateFormatter()
        plain.formatOptions = [.withInternetDateTime]
        return plain.date(from: raw)
    }
}
