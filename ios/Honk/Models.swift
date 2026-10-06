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
    var facts: [Fact]
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
    var whPerMile: Double?
    var energyNote: String?
    var rangeUsed: Double?
    var rangeNote: String?
}

struct Fact: Decodable, Identifiable {
    var id: String { label }
    var label: String
    var value: String
}

struct VehicleBrief: Decodable, Identifiable {
    var id: String { vin }
    var vin: String
    var name: String
    var state: String
    var access: String?
    var selected: Bool
}

struct Grab: Decodable, Identifiable {
    var id: String
    var vin: String
    var takenAt: String
    var state: String
    var summary: String
    var facts: [Fact]
}

struct Garage: Decodable {
    var demo: Bool
    var linked: Bool
    var vehicle: VehicleView?
    var vehicles: [VehicleBrief]
    var grabs: [Grab]
    var snapshotNote: String?
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

struct LogPage: Decodable {
    var charges: [ChargeSession]
    var drains: [DrainSpan]
    var battery: [BatteryPoint]
    var software: [SoftwareRow]
    var note: String
}

struct ChargeSession: Decodable, Identifiable {
    var id: String
    var startedAt: String
    var endedAt: String?
    var socStart: Double?
    var socEnd: Double?
    var energyKwh: Double?
    var minutes: Int
    var kind: String?
    var open: Bool
    var oneReading: Bool
}

struct DrainSpan: Decodable, Identifiable {
    var id: String
    var startedAt: String
    var endedAt: String?
    var socStart: Double?
    var socEnd: Double?
    var rangeStart: Double?
    var rangeEnd: Double?
    var minutes: Int
    var open: Bool
}

struct BatteryPoint: Decodable, Identifiable {
    var id: String { at + "-" + String(odometer) }
    var at: String
    var odometer: Double
    var rangeMi: Double?
    var soc: Double?
}

struct SoftwareRow: Decodable, Identifiable {
    var id: String { version }
    var version: String
    var firstSeen: String
    var lastSeen: String
    var notes: String?
}

struct SoftwarePage: Decodable {
    var software: [SoftwareRow]
    var problem: String?
    var note: String?
}

struct CarAlert: Decodable, Identifiable {
    var id: String { name + at }
    var name: String
    var at: String
    var audience: String?
    var detail: String?
}

struct CarAlertsPage: Decodable {
    var alerts: [CarAlert]
    var problem: String?
    var note: String?
}

struct ChargerSite: Decodable, Identifiable {
    var id: String { kind + name }
    var name: String
    var kind: String
    var distanceMiles: Double?
    var available: Int?
    var stalls: Int?
    var closed: Bool
}

struct ChargersPage: Decodable {
    var sites: [ChargerSite]
    var problem: String?
    var note: String?
}

struct ServicePage: Decodable {
    var facts: [Fact]
    var problem: String?
    var note: String?
}

struct Invite: Decodable, Identifiable {
    var id: String
    var state: String?
    var expiresAt: String?
    var link: String?
}

struct InvitesPage: Decodable {
    var invites: [Invite]
    var problem: String?
    var note: String?
    var message: String?
}

struct CommandResult: Decodable {
    var ok: Bool?
    var message: String?
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

    static func date(_ raw: String?) -> Date? {
        guard let raw else { return nil }
        return parse(raw)
    }

    static func clock(_ raw: String?) -> String {
        guard let date = date(raw) else { return "" }
        return date.formatted(date: .omitted, time: .shortened)
    }

    static func day(_ raw: String?) -> String {
        guard let date = date(raw) else { return "Drive" }
        let weekday = date.formatted(.dateTime.weekday(.abbreviated))
        let month = date.formatted(.dateTime.month(.abbreviated))
        let day = date.formatted(.dateTime.day())
        return "\(weekday) · \(month) \(day)"
    }

    static func span(_ start: String, _ end: String?) -> String {
        let left = clock(start)
        let right = clock(end)
        if right.isEmpty { return left.isEmpty ? "Still out" : "\(left) – now" }
        return "\(left) – \(right)"
    }

    static func duration(_ start: String, _ end: String?) -> String {
        guard let began = date(start) else { return "" }
        let stopped = date(end) ?? Date()
        let minutes = max(1, Int(stopped.timeIntervalSince(began) / 60))
        if minutes < 60 { return "\(minutes) min" }
        return "\(minutes / 60) hr \(minutes % 60) min"
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
