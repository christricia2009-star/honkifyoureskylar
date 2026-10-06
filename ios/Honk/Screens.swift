import MapKit
import SwiftUI

enum HonkTab: String, CaseIterable {
    case garage, map, drives, log, drivers, alerts

    var title: String {
        switch self {
        case .garage: return "Garage"
        case .map: return "Map"
        case .drives: return "Drives"
        case .log: return "Log"
        case .drivers: return "Drivers"
        case .alerts: return "Alerts"
        }
    }

    var symbol: String {
        switch self {
        case .garage: return "parkingsign"
        case .map: return "map"
        case .drives: return "road.lanes"
        case .log: return "chart.bar"
        case .drivers: return "person.2"
        case .alerts: return "bell"
        }
    }
}

struct HonkTabBar: View {
    @Binding var tab: HonkTab

    var body: some View {
        HStack(spacing: 0) {
            ForEach(HonkTab.allCases, id: \.self) { item in
                Button {
                    tab = item
                } label: {
                    VStack(spacing: 2) {
                        Image(systemName: item.symbol)
                        Text(item.title).font(.caption2)
                    }
                    .frame(maxWidth: .infinity)
                    .foregroundStyle(tab == item ? Color(red: 0.72, green: 0.22, blue: 0.42) : .secondary)
                }
                .buttonStyle(.plain)
            }
        }
        .padding(.top, 8)
        .padding(.bottom, 2)
        .background(.ultraThinMaterial)
    }
}

struct RootView: View {
    @Bindable var model: AppModel
    @State private var tab: HonkTab = .garage

    var body: some View {
        Group {
            if model.signedIn {
                VStack(spacing: 0) {
                    Group {
                        switch tab {
                        case .garage: GarageScreen(model: model)
                        case .map: MapScreen(model: model)
                        case .drives: TripsScreen(model: model)
                        case .log: LogScreen(model: model)
                        case .drivers: DriversScreen(model: model)
                        case .alerts: AlertsScreen(model: model)
                        }
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    HonkTabBar(tab: $tab)
                }
            } else {
                WelcomeScreen(model: model)
            }
        }
        .confirmationDialog("Poke it once?", isPresented: Binding(
            get: { model.pending != nil },
            set: { if !$0 { model.dismissConfirm() } }
        ), titleVisibility: .visible) {
            Button("Poke it once") {
                let choice = model.takeConfirm()
                Task { await model.confirm(choice) }
            }
            Button("Leave it", role: .cancel) { model.cancelConfirm() }
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
                        model.signIn()
                    } label: {
                        Label("Sign in with Tesla", systemImage: "key")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(Color(red: 0.72, green: 0.22, blue: 0.42))
                    .disabled(model.busy || model.health?.teslaConfigured == false)

                    PastelCard(tint: HonkColor.lilac) {
                        Text("Sign in opens the Honk server on this Mac, then Tesla. Tesla sends the code back through \(model.health?.redirectUri ?? "the registered redirect"), and the Mac server trades it for a real session.")
                            .font(.footnote)
                    }
                    if model.health == nil {
                        PastelCard(tint: HonkColor.butter) {
                            Text("Nothing is answering \(APIClient.configuredBase.absoluteString). Start the Honk server on this Mac, then come back.")
                                .font(.footnote)
                        }
                    } else if model.health?.teslaConfigured == false {
                        PastelCard(tint: HonkColor.mint) {
                            Text("This server has no Tesla client id yet.")
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
    @State private var showControls = false

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
                            CarSwitcher(model: model)
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
                            if let note = garage.snapshotNote, !note.isEmpty {
                                PastelCard(tint: HonkColor.butter) { Text(note).font(.footnote) }
                            }
                            signalGrid(garage.vehicle)
                            if let facts = garage.vehicle?.facts, !facts.isEmpty {
                                PastelCard(tint: HonkColor.lilac) {
                                    Text("On the car").font(.headline)
                                    ForEach(facts) { fact in
                                        HStack(alignment: .firstTextBaseline) {
                                            Text(fact.label).font(.footnote.weight(.semibold))
                                            Spacer()
                                            Text(fact.value).font(.footnote).multilineTextAlignment(.trailing)
                                        }
                                    }
                                }
                            }
                            Button {
                                showControls = true
                            } label: {
                                Label("Controls", systemImage: "slider.horizontal.3")
                                    .frame(maxWidth: .infinity)
                            }
                            .buttonStyle(.bordered)
                            Button {
                                Task { await model.grabReading() }
                            } label: {
                                Label(grabTitle(garage.vehicle), systemImage: "arrow.down.doc")
                                    .frame(maxWidth: .infinity)
                            }
                            .buttonStyle(.borderedProminent)
                            .tint(Color(red: 0.72, green: 0.22, blue: 0.42))
                            .disabled(model.busy)
                            ReadingHistory(grabs: garage.grabs)
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
            .sheet(isPresented: $showControls) {
                ControlsScreen(model: model)
            }
        }
    }

    private func grabTitle(_ car: VehicleView?) -> String {
        switch car?.state.lowercased() {
        case "asleep", "offline": return "Wake and grab"
        case "online": return "Grab a new reading"
        default: return "Check this car"
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

struct CarSwitcher: View {
    var model: AppModel

    var body: some View {
        if let cars = model.garage?.vehicles, cars.count > 1 {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    ForEach(cars) { car in
                        Button {
                            Task { await model.selectVehicle(car.vin) }
                        } label: {
                            Text(carChip(car))
                                .font(.subheadline.weight(car.selected ? .bold : .regular))
                                .padding(.horizontal, 12)
                                .padding(.vertical, 8)
                                .background(car.selected ? HonkColor.blush : Color.white.opacity(0.8), in: Capsule())
                        }
                        .buttonStyle(.plain)
                        .disabled(model.busy)
                    }
                }
            }
        }
    }
}

private func carChip(_ car: VehicleBrief) -> String {
    var parts = [car.name, car.state]
    if let access = car.access, !access.isEmpty {
        parts.append(access.lowercased())
    }
    return parts.joined(separator: " · ")
}

struct ReadingHistory: View {
    var grabs: [Grab]

    var body: some View {
        PastelCard(tint: HonkColor.butter) {
            Text("Readings").font(.headline)
            if grabs.isEmpty {
                Text("No readings yet. Grab one while the car is online. Parked cars still have battery, lock, tires, sentry, and charge limit.")
                    .font(.footnote)
            }
            ForEach(grabs) { grab in
                VStack(alignment: .leading, spacing: 2) {
                    Text(grab.summary).font(.subheadline.weight(.semibold))
                    Text("\(grab.state) · \(HonkFormat.when(grab.takenAt))")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
    }
}

struct MapScreen: View {
    var model: AppModel
    @State private var position: MapCameraPosition = .automatic

    var body: some View {
        NavigationStack {
            ZStack(alignment: .top) {
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
                CarSwitcher(model: model)
                    .padding(.horizontal, 18)
                    .padding(.top, 8)
            }
            .navigationTitle("Map")
        }
    }
}

struct DriversScreen: View {
    var model: AppModel
    @State private var confirmCreate = false
    @State private var revoke: Invite?

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
                    Section("Invites") {
                        if let problem = model.invites.problem, !problem.isEmpty {
                            Text(problem).font(.footnote)
                        }
                        if let note = model.invites.note, !note.isEmpty {
                            Text(note).font(.footnote)
                        }
                        if model.invites.invites.isEmpty {
                            Text("No invite links.")
                        }
                        ForEach(model.invites.invites) { invite in
                            VStack(alignment: .leading, spacing: 4) {
                                Text(invite.state?.isEmpty == false ? invite.state! : "Invite")
                                    .font(.headline)
                                if let link = invite.link, !link.isEmpty {
                                    Text(link).font(.caption).textSelection(.enabled)
                                }
                                if let expires = invite.expiresAt, !expires.isEmpty {
                                    Text(expires).font(.caption).foregroundStyle(.secondary)
                                }
                                Button("Revoke") { revoke = invite }
                                    .disabled(model.busy)
                            }
                        }
                        if model.invites.problem?.contains("not the owner") != true {
                            Button("Create a one-day invite") { confirmCreate = true }
                                .disabled(model.busy)
                        }
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
            .safeAreaInset(edge: .top) {
                CarSwitcher(model: model).padding(.horizontal, 18).padding(.bottom, 6)
            }
            .navigationTitle("Drivers")
            .toolbar {
                Button("Refresh") {
                    Task {
                        await model.loadDrivers(refresh: true)
                        await model.loadInvites(refresh: true)
                    }
                }
            }
            .task {
                await model.loadDrivers(refresh: false)
                await model.loadInvites(refresh: false)
            }
            .confirmationDialog("Create a one-day invite?", isPresented: $confirmCreate, titleVisibility: .visible) {
                Button("Create the invite") { Task { await model.createInvite() } }
                Button("Leave it", role: .cancel) {}
            } message: {
                Text("Anyone who opens the link can use this car in the Tesla app. The link expires in a day.")
            }
            .confirmationDialog("Revoke this invite?", isPresented: Binding(get: { revoke != nil }, set: { if !$0 { revoke = nil } }), titleVisibility: .visible) {
                Button("Revoke", role: .destructive) {
                    let invite = revoke
                    revoke = nil
                    if let invite { Task { await model.revokeInvite(invite.id) } }
                }
                Button("Leave it", role: .cancel) { revoke = nil }
            }
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
                    Section("From the car") {
                        if !model.carAlertProblem.isEmpty {
                            Text(model.carAlertProblem).font(.footnote)
                        }
                        if model.carAlerts.isEmpty {
                            Text("Tesla's recent alerts show up here after you ask once. Honk keeps the ones it has seen.")
                                .font(.footnote)
                        }
                        ForEach(model.carAlerts) { alert in
                            VStack(alignment: .leading, spacing: 4) {
                                Text(alert.name).font(.headline)
                                if let detail = alert.detail, !detail.isEmpty {
                                    Text(detail).font(.subheadline)
                                }
                                Text(HonkFormat.when(alert.at))
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                        Button("Ask Tesla once") { Task { await model.loadCarAlerts(refresh: true) } }
                            .disabled(model.busy)
                    }
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
                await model.loadCarAlerts(refresh: false)
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
