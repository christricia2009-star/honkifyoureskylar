import MapKit
import SwiftUI

struct RootView: View {
    @Bindable var model: AppModel

    var body: some View {
        Group {
            if model.signedIn {
                TabView {
                    GarageScreen(model: model)
                        .tabItem { Label("Garage", systemImage: "parkingsign") }
                    MapScreen(model: model)
                        .tabItem { Label("Map", systemImage: "map") }
                    TripsScreen(model: model)
                        .tabItem { Label("Trips", systemImage: "road.lanes") }
                    DriversScreen(model: model)
                        .tabItem { Label("Drivers", systemImage: "person.2") }
                    AlertsScreen(model: model)
                        .tabItem { Label("Alerts", systemImage: "bell") }
                }
                .tint(Color(red: 0.72, green: 0.22, blue: 0.42))
            } else {
                WelcomeScreen(model: model)
            }
        }
        .confirmationDialog("One careful honk", isPresented: Binding(
            get: { model.pending != nil },
            set: { if !$0 { model.pending = nil } }
        ), titleVisibility: .visible) {
            Button("Do it once") { Task { await model.confirmPending() } }
            Button("Leave it", role: .cancel) {}
        } message: {
            Text(model.pending?.message ?? "")
        }
    }
}

struct WelcomeScreen: View {
    @Bindable var model: AppModel

    var body: some View {
        ZStack {
            HonkBackground()
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    HornMark(size: 54)
                    Text("Honk if You're Skylar")
                        .font(.largeTitle.weight(.bold))
                        .foregroundStyle(HonkColor.ink)
                    SpeechBubble(text: "One owner. One car. I am not a public Tesla app, and I do not want Skylar's password.")
                    if let banner = model.banner {
                        PastelCard(tint: HonkColor.butter) { Text(banner) }
                    }
                    Button {
                        Task { await model.startDemo() }
                    } label: {
                        Label("Look at the demo", systemImage: "theatermasks")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(Color(red: 0.72, green: 0.22, blue: 0.42))
                    .disabled(model.busy)

                    Button {
                        model.signIn()
                    } label: {
                        Label("Sign in with Tesla", systemImage: "key")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.bordered)
                    .disabled(model.busy || model.health?.teslaConfigured == false)

                    PastelCard(tint: HonkColor.lilac) {
                        Text("Sign in opens the server, then Tesla. The code comes back to \(model.health?.redirectUri ?? "the server's /path"). That only finishes when the Go server is the thing answering that URL.")
                            .font(.footnote)
                    }
                    if model.health?.teslaConfigured == false {
                        PastelCard(tint: HonkColor.mint) {
                            Text("This server has no Tesla client id yet. The demo still drives around the garage.")
                                .font(.footnote)
                        }
                    }
                }
                .padding(22)
            }
        }
    }
}

struct GarageScreen: View {
    @Bindable var model: AppModel
    @State private var showSetup = false

    var body: some View {
        NavigationStack {
            ZStack {
                HonkBackground()
                ScrollView {
                    VStack(alignment: .leading, spacing: 14) {
                        if let banner = model.banner {
                            PastelCard(tint: HonkColor.butter) { Text(banner).font(.footnote) }
                        }
                        if let garage = model.garage {
                            if garage.demo {
                                Text("DEMO CAR")
                                    .font(.caption.weight(.heavy))
                                    .padding(.horizontal, 10)
                                    .padding(.vertical, 4)
                                    .background(HonkColor.butter, in: Capsule())
                            }
                            HStack(spacing: 12) {
                                HornMark()
                                VStack(alignment: .leading) {
                                    Text(garage.vehicle?.name ?? "Empty garage")
                                        .font(.title2.weight(.bold))
                                    Text(stateLine(garage.vehicle))
                                        .font(.subheadline)
                                        .foregroundStyle(.secondary)
                                }
                            }
                            SpeechBubble(text: garage.horn.line)
                            signalGrid(garage.vehicle)
                            if let note = garage.vehicle?.activeDriverNote {
                                PastelCard(tint: HonkColor.lilac) {
                                    Text(note).font(.footnote)
                                }
                            }
                            PastelCard(tint: HonkColor.mint) {
                                Text("This month, about \(HonkFormat.money(garage.usage.estimatedUsd))")
                                    .font(.headline)
                                Text("Credit left, about \(HonkFormat.money(garage.usage.remainingCreditUsd)) of \(HonkFormat.money(garage.usage.monthlyCreditUsd)).")
                                    .font(.subheadline)
                                Text(garage.usage.note)
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        } else {
                            ProgressView("Listening for the car")
                        }
                    }
                    .padding(18)
                }
                .refreshable { await model.peek(confirm: false) }
            }
            .navigationTitle("Garage")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        showSetup = true
                        Task { await model.loadSetup() }
                    } label: {
                        HornMark(size: 18)
                    }
                    .accessibilityLabel("Setup")
                }
            }
            .sheet(isPresented: $showSetup) {
                SetupScreen(model: model)
            }
        }
    }

    private func stateLine(_ car: VehicleView?) -> String {
        guard let car else { return "No car yet" }
        let seen = HonkFormat.when(car.lastSeen)
        return "\(car.state) · \(car.gearLabel) · last heard \(seen)"
    }

    @ViewBuilder
    private func signalGrid(_ car: VehicleView?) -> some View {
        if let car {
            LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 10) {
                fact("Speed", HonkFormat.mph(car.speedMph), HonkColor.blush)
                fact("Battery", car.soc.map { "\(Int($0.rounded()))%" } ?? "—", HonkColor.mint)
                fact("Range", car.estRangeMiles.map { HonkFormat.miles($0) } ?? "—", HonkColor.butter)
                fact("Doors", car.doorSummary, HonkColor.lilac)
                fact("Lock", car.locked == true ? "Locked" : (car.locked == false ? "Unlocked" : "Unknown"), HonkColor.blush)
                fact("Seat", seatLine(car), HonkColor.mint)
            }
        }
    }

    private func seatLine(_ car: VehicleView) -> String {
        let seat = car.driverSeatOccupied == true ? "Occupied" : (car.driverSeatOccupied == false ? "Empty" : "Unknown")
        if car.guestMode == true { return "\(seat), guest mode" }
        return seat
    }

    private func fact(_ title: String, _ value: String, _ tint: Color) -> some View {
        PastelCard(tint: tint) {
            Text(title).font(.caption.weight(.semibold))
            Text(value).font(.headline)
        }
    }
}

struct MapScreen: View {
    var model: AppModel
    @State private var position: MapCameraPosition = .automatic

    var body: some View {
        NavigationStack {
            ZStack {
                HonkBackground()
                if let car = model.garage?.vehicle, let lat = car.latitude, let lng = car.longitude {
                    let coordinate = CLLocationCoordinate2D(latitude: lat, longitude: lng)
                    Map(position: $position) {
                        Annotation(car.name, coordinate: coordinate) {
                            HornMark(size: 22)
                                .padding(8)
                                .background(.white, in: Circle())
                        }
                        if let trip = model.garage?.activeTrip, trip.polyline.count > 1 {
                            MapPolyline(coordinates: trip.polyline.map {
                                CLLocationCoordinate2D(latitude: $0.latitude, longitude: $0.longitude)
                            })
                            .stroke(Color(red: 0.72, green: 0.22, blue: 0.42), lineWidth: 4)
                        }
                    }
                    .mapStyle(.standard)
                    .clipShape(RoundedRectangle(cornerRadius: 24, style: .continuous))
                    .padding(12)
                    .onAppear { position = .region(MKCoordinateRegion(center: coordinate, latitudinalMeters: 2500, longitudinalMeters: 2500)) }
                } else {
                    VStack(spacing: 12) {
                        HornMark()
                        Text("The car has not shared a location yet.")
                            .multilineTextAlignment(.center)
                    }
                    .padding()
                }
            }
            .navigationTitle("Map")
        }
    }
}

struct TripsScreen: View {
    var model: AppModel

    var body: some View {
        NavigationStack {
            ZStack {
                HonkBackground()
                if model.trips.isEmpty {
                    Text("No trips yet. The horn starts one when the car leaves Park.")
                        .padding()
                } else {
                    List(model.trips) { trip in
                        NavigationLink {
                            TripDetail(trip: trip)
                        } label: {
                            VStack(alignment: .leading, spacing: 4) {
                                Text("\(HonkFormat.when(trip.startedAt))")
                                    .font(.headline)
                                Text("\(HonkFormat.miles(trip.distanceMiles)) · max \(HonkFormat.mph(trip.maxSpeedMph))")
                                    .font(.subheadline)
                                if trip.overLimit {
                                    Text(trip.callout ?? "Over the limit.")
                                        .font(.footnote)
                                        .foregroundStyle(Color(red: 0.72, green: 0.22, blue: 0.42))
                                }
                                if trip.pendingClose {
                                    Text("Parked, waiting two minutes to close the trip.")
                                        .font(.caption)
                                }
                            }
                            .padding(.vertical, 4)
                        }
                        .listRowBackground(trip.overLimit ? HonkColor.blush : HonkColor.butter.opacity(0.7))
                    }
                    .scrollContentBackground(.hidden)
                }
            }
            .navigationTitle("Trips")
            .task { await model.loadTrips() }
        }
    }
}

struct TripDetail: View {
    var trip: Trip

    var body: some View {
        ZStack {
            HonkBackground()
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    if let callout = trip.callout, !callout.isEmpty {
                        SpeechBubble(text: callout)
                    }
                    PastelCard(tint: HonkColor.mint) {
                        Text("Max \(HonkFormat.mph(trip.maxSpeedMph))").font(.title3.weight(.bold))
                        Text("Distance \(HonkFormat.miles(trip.distanceMiles))")
                        Text("Limit \(HonkFormat.mph(trip.speedLimitMph))")
                        Text(trip.endedAt == nil ? "Still out" : "Ended \(HonkFormat.when(trip.endedAt))")
                            .font(.footnote)
                        if trip.guestMode == true {
                            Text("Guest mode was on. The horn is not naming the driver.")
                                .font(.footnote)
                        } else if trip.seatOccupied == true {
                            Text("The driver seat was occupied. That is not a name.")
                                .font(.footnote)
                        }
                    }
                    if trip.polyline.count > 1 {
                        Map {
                            MapPolyline(coordinates: trip.polyline.map {
                                CLLocationCoordinate2D(latitude: $0.latitude, longitude: $0.longitude)
                            })
                            .stroke(Color(red: 0.72, green: 0.22, blue: 0.42), lineWidth: 4)
                        }
                        .mapStyle(.standard)
                        .frame(height: 280)
                        .clipShape(RoundedRectangle(cornerRadius: 22, style: .continuous))
                    }
                }
                .padding(18)
            }
        }
        .navigationTitle("Trip")
        .navigationBarTitleDisplayMode(.inline)
    }
}

struct DriversScreen: View {
    var model: AppModel

    var body: some View {
        NavigationStack {
            ZStack {
                HonkBackground()
                List {
                    Section {
                        SpeechBubble(text: model.drivers.note.isEmpty ? "The allow-list is not the person in the seat." : model.drivers.note)
                            .listRowBackground(Color.clear)
                            .listRowInsets(EdgeInsets())
                    }
                    if !model.drivers.problem.isEmpty {
                        Section {
                            Text(model.drivers.problem)
                        }
                        .listRowBackground(HonkColor.butter)
                    }
                    Section("Allow-list") {
                        if model.drivers.drivers.isEmpty {
                            Text("No names on the list.")
                        }
                        ForEach(model.drivers.drivers) { driver in
                            VStack(alignment: .leading, spacing: 4) {
                                Text(driver.name).font(.headline)
                                Text(driver.detail).font(.footnote)
                                if driver.sample {
                                    Text("Sample").font(.caption.weight(.bold))
                                }
                            }
                        }
                    }
                }
                .scrollContentBackground(.hidden)
            }
            .navigationTitle("Drivers")
            .toolbar {
                Button("Refresh") { Task { await model.loadDrivers(refresh: true) } }
            }
            .task { await model.loadDrivers(refresh: false) }
        }
    }
}

struct AlertsScreen: View {
    @Bindable var model: AppModel
    @State private var draft: Settings?

    var body: some View {
        NavigationStack {
            ZStack {
                HonkBackground()
                Form {
                    Section("The horn noticed") {
                        if model.alerts.isEmpty {
                            Text("Quiet so far.")
                        }
                        ForEach(model.alerts) { alert in
                            VStack(alignment: .leading, spacing: 4) {
                                Text(alert.message).font(.body)
                                Text("\(alert.kind) · \(HonkFormat.when(alert.createdAt))")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                            .opacity(alert.read ? 0.55 : 1)
                            .onTapGesture { Task { await model.markRead(alert.id) } }
                        }
                    }
                    if let binding = settingsBinding {
                        Section("Speed cap") {
                            Stepper(value: binding.speedLimitMph, in: 20...130, step: 1) {
                                Text("\(Int(binding.wrappedValue.speedLimitMph.rounded())) mph")
                            }
                        }
                        Section("Curfew") {
                            TextField("Start", text: binding.curfewStart)
                            TextField("End", text: binding.curfewEnd)
                            Text("23:00 through 04:00 means overnight. 04:00 is not inside the window.")
                                .font(.caption)
                        }
                        Section("Home") {
                            Toggle("Watch a home circle", isOn: binding.homeSet)
                            if binding.wrappedValue.homeSet {
                                TextField("Latitude", value: binding.homeLatitude, format: .number)
                                TextField("Longitude", value: binding.homeLongitude, format: .number)
                                Stepper(value: binding.homeRadiusMeters, in: 50...5000, step: 25) {
                                    Text("\(Int(binding.wrappedValue.homeRadiusMeters.rounded())) m")
                                }
                                Button("Use the car's last spot") { useCar(binding) }
                            }
                        }
                        Section("Clock") {
                            TextField("Timezone", text: binding.timezone)
                            Button("Use this phone's timezone") {
                                binding.wrappedValue.timezone = TimeZone.current.identifier
                            }
                        }
                        Section {
                            Button("Save the rules") {
                                Task { await model.save(settings: binding.wrappedValue) }
                            }
                        }
                    }
                }
                .scrollContentBackground(.hidden)
            }
            .navigationTitle("Alerts")
            .task {
                await model.loadAlerts()
                draft = model.settings
            }
            .onChange(of: model.settings?.speedLimitMph) { _, _ in
                if draft == nil { draft = model.settings }
            }
        }
    }

    private var settingsBinding: Binding<Settings>? {
        guard draft != nil else { return nil }
        return Binding(get: { draft! }, set: { draft = $0 })
    }

    private func useCar(_ binding: Binding<Settings>) {
        guard let car = model.garage?.vehicle, let lat = car.latitude, let lng = car.longitude else {
            model.banner = "The car has not shared a spot to use as home."
            return
        }
        binding.wrappedValue.homeLatitude = lat
        binding.wrappedValue.homeLongitude = lng
        binding.wrappedValue.homeSet = true
        if binding.wrappedValue.homeRadiusMeters < 50 {
            binding.wrappedValue.homeRadiusMeters = 250
        }
    }
}

struct SetupScreen: View {
    var model: AppModel
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            ZStack {
                HonkBackground()
                List {
                    if let setup = model.setup {
                        Section {
                            ForEach(setup.items) { item in
                                VStack(alignment: .leading, spacing: 6) {
                                    HStack {
                                        Image(systemName: item.done ? "checkmark.circle.fill" : "circle")
                                        Text(item.title).font(.headline)
                                    }
                                    Text(item.detail).font(.footnote)
                                    if item.id == "virtual_key" && !item.done {
                                        Link("Add the key in the Tesla app", destination: URL(string: setup.pairingUrl)!)
                                        Button("I added the key") { Task { await model.ack("virtual_key") } }
                                    }
                                    if item.id == "billing" && !item.done {
                                        Button("The card and the $10 cap are set") { Task { await model.ack("billing") } }
                                    }
                                }
                                .padding(.vertical, 4)
                            }
                        }
                        .listRowBackground(HonkColor.blush.opacity(0.8))
                        Section("Where things live") {
                            labeled("Redirect", setup.redirectUri)
                            labeled("Public key", setup.publicKeyUrl)
                            labeled("Pairing", setup.pairingUrl)
                            Text(setup.locationNote).font(.footnote)
                            Text(setup.scopes.joined(separator: " ")).font(.caption2)
                        }
                        Section {
                            Button("Send telemetry config") { Task { await model.configureTelemetry() } }
                        } footer: {
                            Text("This is one signed command. It does not start a poll. The Fleet Telemetry hostname is a separate server, not this phone and not the static site.")
                        }
                    } else {
                        ProgressView()
                    }
                }
                .scrollContentBackground(.hidden)
            }
            .navigationTitle("Setup")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close") { dismiss() }
                }
            }
        }
    }

    private func labeled(_ title: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title).font(.caption.weight(.semibold))
            Text(value).font(.footnote).textSelection(.enabled)
        }
    }
}
