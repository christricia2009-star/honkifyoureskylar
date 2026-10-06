import CoreLocation
import Foundation

enum PhonePresence {
    case withTesla
    case walking
    case running
    case cycling
    case otherCar
    case stayedPut
    case noTrail

    var title: String {
        switch self {
        case .withTesla: return "With this Tesla"
        case .walking: return "Walking"
        case .running: return "Running"
        case .cycling: return "On a bike"
        case .otherCar: return "Another car"
        case .stayedPut: return "Phone stayed put"
        case .noTrail: return "No phone trail"
        }
    }
}

struct PhoneVerdict {
    var presence: PhonePresence
    var detail: String
}

struct DriveEvent: Identifiable {
    var id: String
    var tripId: String
    var title: String
    var detail: String
    var at: Date?
}

struct WeekDriveReport {
    var since: Date
    var speeding: [DriveEvent]
    var phoneUse: [DriveEvent]
    var rapidAccel: [DriveEvent]
    var hardBrake: [DriveEvent]

}

enum TripStory {
    static func weekStart(_ now: Date = Date()) -> Date {
        var calendar = Calendar.current
        calendar.firstWeekday = 2
        let parts = calendar.dateComponents([.yearForWeekOfYear, .weekOfYear], from: now)
        return calendar.date(from: parts) ?? now
    }

    static func report(trips: [Trip], fixes: [PhoneFix], opens: [Date], now: Date = Date()) -> WeekDriveReport {
        let since = weekStart(now)
        let week = trips.filter { trip in
            guard let start = HonkFormat.date(trip.startedAt) else { return false }
            return start >= since
        }
        var speeding: [DriveEvent] = []
        var accel: [DriveEvent] = []
        var brake: [DriveEvent] = []
        var verdicts: [String: PhoneVerdict] = [:]
        for trip in week {
            speeding.append(contentsOf: speedingEvents(trip))
            let motion = motionEvents(trip)
            accel.append(contentsOf: motion.accel)
            brake.append(contentsOf: motion.brake)
            verdicts[trip.id] = verdict(trip: trip, fixes: fixes, now: now)
        }
        return WeekDriveReport(
            since: since,
            speeding: speeding,
            phoneUse: phoneEvents(trips: week, opens: opens.filter { $0 >= since }, verdicts: verdicts),
            rapidAccel: accel,
            hardBrake: brake
        )
    }

    static func verdict(trip: Trip, fixes: [PhoneFix], now: Date = Date()) -> PhoneVerdict {
        guard let start = HonkFormat.date(trip.startedAt) else {
            return PhoneVerdict(presence: .noTrail, detail: "This drive has no start time, so the phone trail cannot be lined up with it.")
        }
        let end = HonkFormat.date(trip.endedAt) ?? now
        let during = fixes.filter { $0.at >= start.addingTimeInterval(-90) && $0.at <= end.addingTimeInterval(90) }
        if during.isEmpty {
            if let prior = fixes.last(where: { $0.at <= start && start.timeIntervalSince($0.at) < 45 * 60 }),
               prior.speedMph < 2,
               nearestMeters(prior, trip) > 300 {
                return PhoneVerdict(presence: .stayedPut, detail: "This phone's last point before the drive was still, and it was not on the Tesla's path.")
            }
            return PhoneVerdict(presence: .noTrail, detail: "This phone did not record a location during this drive. Honk only knows about the phone running the app.")
        }
        let distances = during.map { nearestMeters($0, trip) }
        let closeCount = distances.filter { $0 <= 180 }.count
        let close = Double(closeCount) / Double(during.count) >= 0.6
        let activity = dominant(during)
        let phoneSpeed = median(during.map(\.speedMph))
        if close && activity != "walking" && activity != "running" && activity != "cycling" {
            return PhoneVerdict(presence: .withTesla, detail: "This phone followed the Tesla's path. The motion fits a person riding in the car.")
        }
        if close && (activity == "walking" || activity == "running" || activity == "cycling") && phoneSpeed + 8 < carSpeed(trip) {
            return PhoneVerdict(presence: presence(for: activity), detail: "The path passed near the Tesla, but this phone was moving like a person on foot or on a bike.")
        }
        if close {
            return PhoneVerdict(presence: .withTesla, detail: "This phone stayed with the Tesla's path for this drive.")
        }
        let presence = presence(for: activity == "unknown" ? band(phoneSpeed) : activity)
        return PhoneVerdict(presence: presence, detail: detail(for: presence))
    }

    static func speedingEvents(_ trip: Trip) -> [DriveEvent] {
        let limit = trip.speedLimitMph > 0 ? trip.speedLimitMph : 75
        var events: [DriveEvent] = []
        var over = false
        for point in trip.polyline {
            let fast = (point.speedMph ?? 0) > limit
            if fast && !over {
                events.append(DriveEvent(
                    id: "speed-\(trip.id)-\(events.count)",
                    tripId: trip.id,
                    title: "Over \(Int(limit.rounded())) mph",
                    detail: "Tesla reported \(Int((point.speedMph ?? trip.maxSpeedMph).rounded())) mph.",
                    at: HonkFormat.date(point.at)
                ))
            }
            over = fast
        }
        if events.isEmpty && trip.overLimit {
            events.append(DriveEvent(
                id: "speed-\(trip.id)-max",
                tripId: trip.id,
                title: "Over \(Int(limit.rounded())) mph",
                detail: "The car reached \(Int(trip.maxSpeedMph.rounded())) mph.",
                at: HonkFormat.date(trip.startedAt)
            ))
        }
        return events
    }

    static func motionEvents(_ trip: Trip) -> (accel: [DriveEvent], brake: [DriveEvent]) {
        var accel: [DriveEvent] = []
        var brake: [DriveEvent] = []
        let points = trip.polyline
        guard points.count > 1 else { return ([], []) }
        for index in 1..<points.count {
            let previous = points[index - 1]
            let point = points[index]
            guard let from = previous.speedMph, let to = point.speedMph,
                  let began = HonkFormat.date(previous.at), let ended = HonkFormat.date(point.at) else { continue }
            let seconds = ended.timeIntervalSince(began)
            guard seconds > 0, seconds < 30 else { continue }
            let delta = to - from
            let rate = abs(delta) / seconds
            if delta >= 15, rate >= 4 {
                accel.append(DriveEvent(
                    id: "accel-\(trip.id)-\(accel.count)",
                    tripId: trip.id,
                    title: "Rapid acceleration",
                    detail: "Tesla speed rose \(Int(delta.rounded())) mph in \(Int(seconds.rounded())) seconds.",
                    at: ended
                ))
            }
            if delta <= -15, rate >= 3.5 {
                brake.append(DriveEvent(
                    id: "brake-\(trip.id)-\(brake.count)",
                    tripId: trip.id,
                    title: "Hard braking",
                    detail: "Tesla speed fell \(Int((-delta).rounded())) mph in \(Int(seconds.rounded())) seconds.",
                    at: ended
                ))
            }
        }
        return (accel, brake)
    }
}

private func phoneEvents(trips: [Trip], opens: [Date], verdicts: [String: PhoneVerdict]) -> [DriveEvent] {
    var events: [DriveEvent] = []
    var last: Date?
    for open in opens.sorted() {
        if let last, open.timeIntervalSince(last) < 120 { continue }
        guard let trip = trips.first(where: { trip in
            guard verdicts[trip.id]?.presence == .withTesla else { return false }
            guard let start = HonkFormat.date(trip.startedAt) else { return false }
            let end = HonkFormat.date(trip.endedAt) ?? open
            return open >= start.addingTimeInterval(-60) && open <= end.addingTimeInterval(60)
        }) else { continue }
        last = open
        events.append(DriveEvent(
            id: "phone-\(trip.id)-\(Int(open.timeIntervalSince1970))",
            tripId: trip.id,
            title: "This phone was open",
            detail: "It was open while it was moving with this Tesla. Honk cannot see other apps.",
            at: open
        ))
    }
    return events
}

private func presence(for activity: String) -> PhonePresence {
    switch activity {
    case "walking": return .walking
    case "running": return .running
    case "cycling": return .cycling
    case "automotive": return .otherCar
    case "stationary": return .stayedPut
    default: return .otherCar
    }
}

private func band(_ mph: Double) -> String {
    if mph < 1 { return "stationary" }
    if mph < 4.5 { return "walking" }
    if mph < 10 { return "running" }
    if mph < 18 { return "cycling" }
    return "automotive"
}

private func detail(for presence: PhonePresence) -> String {
    switch presence {
    case .withTesla:
        return "This phone stayed with the Tesla."
    case .walking:
        return "This phone was walking, away from the Tesla's path."
    case .running:
        return "This phone was running, away from the Tesla's path."
    case .cycling:
        return "This phone was on a bike, away from the Tesla's path."
    case .otherCar:
        return "This phone was moving in a vehicle, but not along this Tesla's path."
    case .stayedPut:
        return "This phone stayed put while the Tesla moved."
    case .noTrail:
        return "This phone has no location for this drive."
    }
}

private func dominant(_ fixes: [PhoneFix]) -> String {
    var counts: [String: Int] = [:]
    for fix in fixes where fix.activity != "unknown" {
        counts[fix.activity, default: 0] += 1
    }
    return counts.max { $0.value < $1.value }?.key ?? "unknown"
}

private func carSpeed(_ trip: Trip) -> Double {
    let speeds = trip.polyline.compactMap(\.speedMph).filter { $0 > 0 }
    return median(speeds)
}

private func median(_ values: [Double]) -> Double {
    let sorted = values.sorted()
    guard !sorted.isEmpty else { return 0 }
    let mid = sorted.count / 2
    if sorted.count.isMultiple(of: 2) { return (sorted[mid - 1] + sorted[mid]) / 2 }
    return sorted[mid]
}

private func nearestMeters(_ fix: PhoneFix, _ trip: Trip) -> Double {
    guard !trip.polyline.isEmpty else { return .greatestFiniteMagnitude }
    var timed = Double.greatestFiniteMagnitude
    var any = Double.greatestFiniteMagnitude
    var sawTime = false
    for point in trip.polyline {
        let meters = metersBetween(fix.latitude, fix.longitude, point.latitude, point.longitude)
        any = min(any, meters)
        if let at = HonkFormat.date(point.at) {
            sawTime = true
            if abs(at.timeIntervalSince(fix.at)) <= 180 {
                timed = min(timed, meters)
            }
        }
    }
    if sawTime && timed < .greatestFiniteMagnitude { return timed }
    return any
}

func metersBetween(_ lat1: Double, _ lng1: Double, _ lat2: Double, _ lng2: Double) -> Double {
    let radius = 6_371_000.0
    let dLat = (lat2 - lat1) * .pi / 180
    let dLng = (lng2 - lng1) * .pi / 180
    let a = sin(dLat / 2) * sin(dLat / 2)
        + cos(lat1 * .pi / 180) * cos(lat2 * .pi / 180) * sin(dLng / 2) * sin(dLng / 2)
    return 2 * radius * asin(min(1, sqrt(a)))
}

@MainActor
@Observable
final class PlaceBook {
    static let shared = PlaceBook()

    private(set) var names: [String: String] = [:]
    private var queue: [(CLLocation, String)] = []
    private var queued: Set<String> = []
    private var running = false

    private init() {
        if let stored = UserDefaults.standard.dictionary(forKey: "honk.places") as? [String: String] {
            names = stored
        }
    }

    func label(latitude: Double, longitude: Double) -> String {
        let key = String(format: "%.4f,%.4f", latitude, longitude)
        if let name = names[key] { return name }
        if queued.insert(key).inserted {
            queue.append((CLLocation(latitude: latitude, longitude: longitude), key))
            Task { await pump() }
        }
        return "Finding the place"
    }

    private func pump() async {
        guard !running else { return }
        running = true
        defer { running = false }
        let geocoder = CLGeocoder()
        while !queue.isEmpty {
            let (location, key) = queue.removeFirst()
            if names[key] != nil { continue }
            let line: String
            if let marks = try? await geocoder.reverseGeocodeLocation(location), let mark = marks.first {
                line = Self.line(mark)
            } else {
                line = "Unnamed place"
            }
            names[key] = line
            UserDefaults.standard.set(names, forKey: "honk.places")
        }
    }

    private static func line(_ mark: CLPlacemark) -> String {
        if let number = mark.subThoroughfare, let street = mark.thoroughfare {
            return "\(number) \(street)"
        }
        if let street = mark.thoroughfare { return street }
        if let name = mark.name, !name.isEmpty { return name }
        return mark.locality ?? "Unnamed place"
    }
}
