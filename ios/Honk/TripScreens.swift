import MapKit
import SwiftUI

private let routePurple = Color(red: 0.42, green: 0.27, blue: 0.86)

struct TripsScreen: View {
    var model: AppModel
    @ObservedObject var trail = PhoneTrail.shared
    var places = PlaceBook.shared

    var body: some View {
        let report = TripStory.report(trips: model.trips, fixes: trail.fixes, opens: trail.opens)
        NavigationStack {
            ZStack {
                HonkBackground()
                ScrollView {
                    VStack(alignment: .leading, spacing: 14) {
                        CarSwitcher(model: model)
                        header
                        WeekReportCard(report: report)
                        phoneCard
                        if model.trips.isEmpty {
                            Text("No drives yet. One starts when this Tesla leaves Park. This phone's trail is kept so a later drive can be matched or ruled out.")
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                        }
                        ForEach(model.trips) { trip in
                            NavigationLink {
                                TripDetail(trip: trip)
                            } label: {
                                DriveCard(trip: trip, verdict: TripStory.verdict(trip: trip, fixes: trail.fixes), places: places)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(18)
                }
            }
            .navigationTitle("Drives")
            .task {
                PhoneTrail.shared.start()
                await model.loadTrips()
            }
        }
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(model.garage?.vehicle?.name ?? "This car")
                .font(.largeTitle.weight(.bold))
                .foregroundStyle(HonkColor.ink)
            Spacer()
            if let soc = model.garage?.vehicle?.soc {
                Text("Car \(Int(soc.rounded()))%")
                    .font(.subheadline.weight(.semibold))
                    .padding(.horizontal, 10)
                    .padding(.vertical, 6)
                    .background(Color.white, in: Capsule())
            }
        }
    }

    private var phoneCard: some View {
        PastelCard(tint: Color.white.opacity(0.92)) {
            Text(trail.sharing ? "This phone's GPS is on" : "This phone's GPS is off")
                .font(.headline)
            Text("Honk lines this phone up with the Tesla. If the phone walks, runs, bikes, stays home, or rides in a different car, that drive is not counted as someone in this Tesla. It only knows about the phone that has the app.")
                .font(.footnote)
                .foregroundStyle(.secondary)
            if trail.denied {
                Text("Location is off for Honk. Turn it on in Settings to compare this phone with the car.")
                    .font(.footnote)
            } else if !trail.sharing {
                Button("Use this phone's GPS") { trail.start() }
                    .buttonStyle(.borderedProminent)
                    .tint(routePurple)
            }
        }
    }
}

private struct WeekReportCard: View {
    var report: WeekDriveReport

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("This week's drives")
                .font(.title3.weight(.bold))
            Text("Since \(report.since.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day()))")
                .font(.subheadline)
                .foregroundStyle(.secondary)
            reportLink("Speeding", count: report.speeding.count, tint: Color(red: 0.93, green: 0.33, blue: 0.42), symbol: "speedometer", events: report.speeding, blurb: "Times Tesla speed went over the limit you set. The count is open.")
            reportLink("Distracted", count: report.phoneUse.count, tint: Color(red: 0.29, green: 0.62, blue: 0.96), symbol: "iphone", events: report.phoneUse, blurb: "Times this phone was opened while it was moving with this Tesla. Honk cannot see other apps, and a phone that stayed home does not count.")
            reportLink("Rapid Accel", count: report.rapidAccel.count, tint: Color(red: 0.62, green: 0.36, blue: 0.95), symbol: "bolt.fill", events: report.rapidAccel, blurb: "Sharp speed gains from Tesla's own speed samples. A slow climb is not counted.")
            reportLink("Hard Braking", count: report.hardBrake.count, tint: Color(red: 0.95, green: 0.72, blue: 0.18), symbol: "arrow.down.to.line", events: report.hardBrake, blurb: "Sharp speed drops from Tesla's own speed samples.")
        }
        .padding(16)
        .background(Color.white, in: RoundedRectangle(cornerRadius: 28, style: .continuous))
    }

    private func reportLink(_ title: String, count: Int, tint: Color, symbol: String, events: [DriveEvent], blurb: String) -> some View {
        NavigationLink {
            ReportDetail(title: title, blurb: blurb, events: events)
        } label: {
            HStack(spacing: 12) {
                Image(systemName: symbol)
                    .font(.body.weight(.bold))
                    .foregroundStyle(.white)
                    .frame(width: 36, height: 36)
                    .background(tint, in: Circle())
                Text(title)
                    .font(.body.weight(.semibold))
                    .foregroundStyle(HonkColor.ink)
                Spacer()
                Text("\(count)")
                    .font(.subheadline.weight(.bold))
                    .foregroundStyle(HonkColor.ink)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 4)
                    .background(tint.opacity(0.18), in: Capsule())
                Image(systemName: "chevron.right")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(.secondary)
            }
        }
        .buttonStyle(.plain)
    }
}

private struct ReportDetail: View {
    var title: String
    var blurb: String
    var events: [DriveEvent]

    var body: some View {
        ZStack {
            HonkBackground()
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    Text(blurb)
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                    if events.isEmpty {
                        Text("None this week.")
                            .font(.headline)
                    }
                    ForEach(events) { event in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(event.title).font(.headline)
                            Text(event.detail).font(.subheadline)
                            if let at = event.at {
                                Text(at.formatted(date: .abbreviated, time: .shortened))
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(14)
                        .background(Color.white, in: RoundedRectangle(cornerRadius: 18, style: .continuous))
                    }
                }
                .padding(18)
            }
        }
        .navigationTitle(title)
        .navigationBarTitleDisplayMode(.inline)
    }
}

private struct DriveCard: View {
    var trip: Trip
    var verdict: PhoneVerdict
    var places: PlaceBook

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(HonkFormat.day(trip.startedAt))
                .font(.title3.weight(.bold))
                .foregroundStyle(HonkColor.ink)
            DriveMap(trip: trip)
                .frame(height: 150)
                .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
            Text(verdict.presence.title)
                .font(.caption.weight(.bold))
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
                .background(HonkColor.lilac, in: Capsule())
            Text(routeLine)
                .font(.headline)
                .foregroundStyle(HonkColor.ink)
            Text("\(HonkFormat.span(trip.startedAt, trip.endedAt)) · \(HonkFormat.miles(trip.distanceMiles))")
                .font(.subheadline)
                .foregroundStyle(.secondary)
            if let economy = economyLine(trip) {
                Text(economy)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(HonkColor.ink)
            }
        }
        .padding(14)
        .background(Color.white, in: RoundedRectangle(cornerRadius: 28, style: .continuous))
    }

    private var routeLine: String {
        guard let start = trip.polyline.first, let end = trip.polyline.last else {
            return "Route not recorded"
        }
        let from = places.label(latitude: start.latitude, longitude: start.longitude)
        let to = places.label(latitude: end.latitude, longitude: end.longitude)
        return "\(from) → \(to)"
    }
}

struct TripDetail: View {
    var trip: Trip
    @ObservedObject var trail = PhoneTrail.shared
    var places = PlaceBook.shared

    var body: some View {
        let verdict = TripStory.verdict(trip: trip, fixes: trail.fixes)
        ZStack {
            HonkBackground()
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    Text(HonkFormat.day(trip.startedAt))
                        .font(.largeTitle.weight(.bold))
                    if let end = trip.polyline.last {
                        Text("At \(places.label(latitude: end.latitude, longitude: end.longitude))")
                            .font(.title3.weight(.semibold))
                    }
                    Text(HonkFormat.span(trip.startedAt, trip.endedAt))
                        .font(.title3)
                        .foregroundStyle(.secondary)
                    Label(HonkFormat.duration(trip.startedAt, trip.endedAt), systemImage: "clock")
                        .font(.headline)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 8)
                        .background(Color.white, in: Capsule())
                    DriveMap(trip: trip)
                        .frame(height: 280)
                        .clipShape(RoundedRectangle(cornerRadius: 24, style: .continuous))
                    VStack(alignment: .leading, spacing: 6) {
                        Text(routeLine)
                            .font(.title3.weight(.bold))
                        Text("\(HonkFormat.span(trip.startedAt, trip.endedAt)) · \(HonkFormat.miles(trip.distanceMiles))")
                            .foregroundStyle(.secondary)
                        Text("Max \(HonkFormat.mph(trip.maxSpeedMph)) · limit \(HonkFormat.mph(trip.speedLimitMph))")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                        if let economy = economyLine(trip) {
                            Text(economy).font(.subheadline.weight(.semibold))
                        }
                        if let note = trip.energyNote, !note.isEmpty {
                            Text(note).font(.caption).foregroundStyle(.secondary)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(16)
                    .background(Color.white, in: RoundedRectangle(cornerRadius: 24, style: .continuous))
                    PastelCard(tint: HonkColor.lilac) {
                        Text(verdict.presence.title).font(.headline)
                        Text(verdict.detail).font(.footnote)
                    }
                    if trip.guestMode == true {
                        Text("Guest mode was on. The horn is not naming the driver.")
                            .font(.footnote)
                    } else if trip.seatOccupied == true {
                        Text("The driver seat was occupied. That is not a name.")
                            .font(.footnote)
                    }
                    if trip.pendingClose {
                        Text("Parked, waiting two minutes to close the trip.")
                            .font(.caption)
                    }
                    if let callout = trip.callout, !callout.isEmpty {
                        SpeechBubble(text: callout)
                    }
                }
                .padding(18)
            }
        }
        .navigationTitle("Drive")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var routeLine: String {
        guard let start = trip.polyline.first, let end = trip.polyline.last else {
            return "Route not recorded"
        }
        return "\(places.label(latitude: start.latitude, longitude: start.longitude)) → \(places.label(latitude: end.latitude, longitude: end.longitude))"
    }
}

private func economyLine(_ trip: Trip) -> String? {
    var parts: [String] = []
    if let wh = trip.whPerMile {
        parts.append("\(Int(wh.rounded())) Wh/mi")
    }
    if let used = trip.rangeUsed {
        let label = trip.rangeNote?.isEmpty == false ? trip.rangeNote! : "Range used"
        parts.append(String(format: "%.1f mi · %@", used, label))
    }
    return parts.isEmpty ? nil : parts.joined(separator: " · ")
}

private struct DriveMap: View {
    var trip: Trip

    var body: some View {
        let coordinates = trip.polyline.map {
            CLLocationCoordinate2D(latitude: $0.latitude, longitude: $0.longitude)
        }
        Map {
            if coordinates.count > 1 {
                MapPolyline(coordinates: coordinates)
                    .stroke(routePurple, lineWidth: 5)
            }
            if let start = coordinates.first {
                Annotation("Start", coordinate: start) {
                    Circle()
                        .fill(routePurple)
                        .frame(width: 12, height: 12)
                        .overlay(Circle().stroke(.white, lineWidth: 2))
                }
            }
            if let end = coordinates.last, coordinates.count > 1 {
                Annotation("End", coordinate: end) {
                    Image(systemName: "flag.fill")
                        .font(.caption.weight(.bold))
                        .foregroundStyle(routePurple)
                        .padding(6)
                        .background(.white, in: Circle())
                }
            }
        }
        .mapStyle(.standard(elevation: .flat))
        .allowsHitTesting(false)
    }
}
