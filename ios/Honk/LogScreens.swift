import SwiftUI

enum LogSection: String, CaseIterable, Identifiable {
    case charges = "Charges"
    case drain = "Drain"
    case battery = "Battery"
    case software = "Software"
    case chargers = "Chargers"
    case service = "Service"
    var id: String { rawValue }
}

struct LogScreen: View {
    var model: AppModel
    @State private var section: LogSection = .charges

    var body: some View {
        NavigationStack {
            ZStack {
                HonkBackground()
                ScrollView {
                    VStack(alignment: .leading, spacing: 14) {
                        CarSwitcher(model: model)
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack(spacing: 8) {
                                ForEach(LogSection.allCases) { item in
                                    Button(item.rawValue) { section = item }
                                        .font(.subheadline.weight(section == item ? .bold : .regular))
                                        .padding(.horizontal, 12)
                                        .padding(.vertical, 8)
                                        .background(section == item ? HonkColor.blush : Color.white.opacity(0.85), in: Capsule())
                                        .buttonStyle(.plain)
                                }
                            }
                        }
                        if let note = model.log?.note, !note.isEmpty {
                            Text(note)
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                        }
                        sectionBody
                    }
                    .padding(18)
                }
                .refreshable { await reload(askTesla: false) }
            }
            .navigationTitle("Log")
            .task { await model.loadLog() }
        }
    }

    @ViewBuilder
    private var sectionBody: some View {
        switch section {
        case .charges:
            charges
        case .drain:
            drains
        case .battery:
            battery
        case .software:
            software
        case .chargers:
            chargerList
        case .service:
            serviceList
        }
    }

    private var charges: some View {
        VStack(alignment: .leading, spacing: 10) {
            if model.log?.charges.isEmpty != false {
                Text("No charges yet. A session appears when a reading or the stream sees the car charging, then unplugged or finished.")
                    .font(.footnote)
            }
            ForEach(model.log?.charges ?? []) { charge in
                PastelCard(tint: HonkColor.mint) {
                    Text(HonkFormat.day(charge.startedAt)).font(.headline)
                    Text(chargeLine(charge)).font(.subheadline)
                    if charge.oneReading {
                        Text("Seen in one reading. The start was before Honk was watching.")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
        }
    }

    private var drains: some View {
        VStack(alignment: .leading, spacing: 10) {
            if model.log?.drains.isEmpty != false {
                Text("No parked drain yet. Honk counts a drop while the car stays in Park and is not charging.")
                    .font(.footnote)
            }
            ForEach(model.log?.drains ?? []) { drain in
                PastelCard(tint: HonkColor.butter) {
                    Text(drain.open ? "Still parked" : HonkFormat.day(drain.startedAt)).font(.headline)
                    Text(drainLine(drain)).font(.subheadline)
                }
            }
        }
    }

    private var battery: some View {
        VStack(alignment: .leading, spacing: 10) {
            let points = model.log?.battery ?? []
            if points.count < 2 {
                Text("The battery line needs two readings with an odometer. Honk keeps a point about every 5 miles, or once a day.")
                    .font(.footnote)
            } else {
                RangeChart(points: points)
                    .frame(height: 140)
                    .padding(8)
                    .background(Color.white, in: RoundedRectangle(cornerRadius: 18, style: .continuous))
            }
            ForEach(points) { point in
                HStack {
                    Text(HonkFormat.day(point.at))
                    Spacer()
                    Text(batteryLine(point))
                        .foregroundStyle(.secondary)
                }
                .font(.subheadline)
            }
        }
    }

    private var software: some View {
        VStack(alignment: .leading, spacing: 10) {
            Button("Ask Tesla for release notes") {
                Task { await model.loadSoftware(refresh: true) }
            }
            .buttonStyle(.bordered)
            .disabled(model.busy)
            if model.log?.software.isEmpty != false {
                Text("No version yet. It shows up on a reading, or when you ask Tesla for the notes.")
                    .font(.footnote)
            }
            ForEach(model.log?.software ?? []) { row in
                PastelCard(tint: HonkColor.lilac) {
                    Text(row.version).font(.headline)
                    Text("Seen \(HonkFormat.when(row.firstSeen))")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    if let notes = row.notes, !notes.isEmpty {
                        Text(notes).font(.footnote)
                    }
                }
            }
        }
    }

    private var chargerList: some View {
        VStack(alignment: .leading, spacing: 10) {
            Button("Ask Tesla once") { Task { await model.loadChargers(refresh: true) } }
                .buttonStyle(.bordered)
                .disabled(model.busy)
            if let note = model.chargers.note, !note.isEmpty {
                Text(note).font(.footnote).foregroundStyle(.secondary)
            }
            if let problem = model.chargers.problem, !problem.isEmpty {
                Text(problem).font(.footnote)
            }
            if model.chargers.sites.isEmpty {
                Text("Nearby chargers come from the car's current spot. The car has to be online.")
                    .font(.footnote)
            }
            ForEach(model.chargers.sites) { site in
                PastelCard(tint: Color.white.opacity(0.92)) {
                    Text(site.name).font(.headline)
                    Text(chargerLine(site)).font(.subheadline).foregroundStyle(.secondary)
                }
            }
        }
        .task { await model.loadChargers(refresh: false) }
    }

    private var serviceList: some View {
        VStack(alignment: .leading, spacing: 10) {
            Button("Ask Tesla once") { Task { await model.loadService(refresh: true) } }
                .buttonStyle(.bordered)
                .disabled(model.busy)
            if let note = model.service.note, !note.isEmpty {
                Text(note).font(.footnote).foregroundStyle(.secondary)
            }
            if let problem = model.service.problem, !problem.isEmpty {
                Text(problem).font(.footnote)
            }
            if model.service.facts.isEmpty {
                Text("Service status is whatever Tesla has open on this car. It is not a repair history.")
                    .font(.footnote)
            }
            ForEach(model.service.facts) { fact in
                HStack {
                    Text(fact.label).font(.subheadline.weight(.semibold))
                    Spacer()
                    Text(fact.value).font(.subheadline)
                }
            }
        }
        .task { await model.loadService(refresh: false) }
    }

    private func reload(askTesla: Bool) async {
        await model.loadLog()
        if section == .chargers { await model.loadChargers(refresh: askTesla) }
        if section == .service { await model.loadService(refresh: askTesla) }
    }

    private func chargeLine(_ charge: ChargeSession) -> String {
        var parts: [String] = []
        if let kind = charge.kind, !kind.isEmpty { parts.append(kind) }
        if charge.open { parts.append("charging now") }
        parts.append(charge.minutes < 1 ? "under a minute" : "\(charge.minutes) min")
        if let start = charge.socStart, let end = charge.socEnd {
            parts.append("\(Int(start.rounded()))% → \(Int(end.rounded()))%")
        }
        if let energy = charge.energyKwh {
            parts.append(String(format: "%.1f kWh added", energy))
        }
        return parts.joined(separator: " · ")
    }

    private func drainLine(_ drain: DrainSpan) -> String {
        var parts = ["\(max(drain.minutes, 0)) min parked"]
        if let start = drain.socStart, let end = drain.socEnd {
            let drop = start - end
            parts.append(String(format: "%.0f%% → %.0f%% (%+.0f)", start, end, -drop))
        }
        if let start = drain.rangeStart, let end = drain.rangeEnd {
            parts.append(String(format: "%.0f → %.0f mi", start, end))
        }
        return parts.joined(separator: " · ")
    }

    private func batteryLine(_ point: BatteryPoint) -> String {
        var parts = [String(format: "%.0f mi", point.odometer)]
        if let range = point.rangeMi { parts.append(String(format: "%.0f mi range", range)) }
        if let soc = point.soc { parts.append("\(Int(soc.rounded()))%") }
        return parts.joined(separator: " · ")
    }

    private func chargerLine(_ site: ChargerSite) -> String {
        var parts = [site.kind]
        if let miles = site.distanceMiles { parts.append(String(format: "%.1f mi", miles)) }
        if let available = site.available, let stalls = site.stalls {
            parts.append("\(available) of \(stalls) open")
        }
        if site.closed { parts.append("closed") }
        return parts.joined(separator: " · ")
    }
}

private struct RangeChart: View {
    var points: [BatteryPoint]

    var body: some View {
        let samples = points.reversed().filter { $0.rangeMi != nil }
        GeometryReader { geo in
            if samples.count >= 2 {
                Path { path in
                    let ranges = samples.compactMap(\.rangeMi)
                    let low = ranges.min() ?? 0
                    let high = max(ranges.max() ?? 1, low + 1)
                    for (index, point) in samples.enumerated() {
                        guard let range = point.rangeMi else { continue }
                        let x = geo.size.width * CGFloat(index) / CGFloat(samples.count - 1)
                        let y = geo.size.height * (1 - CGFloat((range - low) / (high - low)))
                        if index == 0 { path.move(to: CGPoint(x: x, y: y)) } else { path.addLine(to: CGPoint(x: x, y: y)) }
                    }
                }
                .stroke(Color(red: 0.42, green: 0.27, blue: 0.86), lineWidth: 3)
            }
        }
    }
}

struct ControlsScreen: View {
    var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var cabinF = 70.0
    @State private var chargeLimit = 80.0
    @State private var speedCap = 70.0
    @State private var pin = ""
    @State private var valetCode = ""
    @State private var chargeStart = Calendar.current.date(bySettingHour: 0, minute: 0, second: 0, of: Date()) ?? Date()
    @State private var chargeEnd = Calendar.current.date(bySettingHour: 6, minute: 0, second: 0, of: Date()) ?? Date()
    @State private var readyAt = Calendar.current.date(bySettingHour: 7, minute: 0, second: 0, of: Date()) ?? Date()
    @State private var pending: ControlSend?

    var body: some View {
        NavigationStack {
            ZStack {
                HonkBackground()
                Form {
                    Section {
                        Text("These go to the selected car. It has to be online, and Honk's key has to be on the car. A sleeping car is left alone. Codes are sent once and not saved.")
                            .font(.footnote)
                    }
                    Section("Doors and lights") {
                        commandButton("Lock") { ControlSend(title: "Lock the car?", body: ["name": "door_lock"]) }
                        commandButton("Unlock") { ControlSend(title: "Unlock the car?", body: ["name": "door_unlock"]) }
                        commandButton("Honk the horn") { ControlSend(title: "Honk the car's horn?", body: ["name": "honk_horn"]) }
                        commandButton("Flash the lights") { ControlSend(title: "Flash the lights?", body: ["name": "flash_lights"]) }
                        commandButton("Open the frunk") { ControlSend(title: "Open the front trunk?", body: ["name": "actuate_trunk", "whichTrunk": "front"]) }
                        commandButton("Open the trunk") { ControlSend(title: "Open the rear trunk?", body: ["name": "actuate_trunk", "whichTrunk": "rear"]) }
                        commandButton("Vent the windows") { ControlSend(title: "Vent the windows?", body: ["name": "window_control", "window": "vent"]) }
                        commandButton("Close the windows") { ControlSend(title: "Close the windows?", body: ["name": "window_control", "window": "close"]) }
                    }
                    Section("Climate") {
                        Stepper(value: $cabinF, in: 60...82, step: 1) {
                            Text("Cabin \(Int(cabinF))°F")
                        }
                        commandButton("Set the cabin temperature") {
                            let c = ((cabinF - 32) * 5 / 9 * 2).rounded() / 2
                            return ControlSend(title: "Set the cabin to \(Int(cabinF))°F?", body: ["name": "set_temps", "driverTemp": c, "passengerTemp": c])
                        }
                        commandButton("Climate on") { ControlSend(title: "Turn climate on?", body: ["name": "auto_conditioning_start"]) }
                        commandButton("Climate off") { ControlSend(title: "Turn climate off?", body: ["name": "auto_conditioning_stop"]) }
                        commandButton("Cabin overheat on") { ControlSend(title: "Turn cabin overheat protection on?", body: ["name": "set_cabin_overheat_protection", "on": true, "fanOnly": false]) }
                        commandButton("Cabin overheat off") { ControlSend(title: "Turn cabin overheat protection off?", body: ["name": "set_cabin_overheat_protection", "on": false, "fanOnly": false]) }
                    }
                    Section("Charge") {
                        commandButton("Start charging") { ControlSend(title: "Start charging?", body: ["name": "charge_start"]) }
                        commandButton("Stop charging") { ControlSend(title: "Stop charging?", body: ["name": "charge_stop"]) }
                        Stepper(value: $chargeLimit, in: 50...100, step: 5) {
                            Text("Limit \(Int(chargeLimit))%")
                        }
                        commandButton("Set the charge limit") {
                            ControlSend(title: "Set the charge limit to \(Int(chargeLimit))%?", body: ["name": "set_charge_limit", "percent": chargeLimit])
                        }
                        DatePicker("Charge from", selection: $chargeStart, displayedComponents: .hourAndMinute)
                        DatePicker("Until", selection: $chargeEnd, displayedComponents: .hourAndMinute)
                        commandButton("Save the charge schedule") {
                            ControlSend(title: "Save a daily charge schedule at the car's last spot?", body: [
                                "name": "add_charge_schedule",
                                "startMinutes": minutes(chargeStart),
                                "endMinutes": minutes(chargeEnd),
                            ])
                        }
                        DatePicker("Cabin ready", selection: $readyAt, displayedComponents: .hourAndMinute)
                        commandButton("Save the precondition time") {
                            ControlSend(title: "Save a daily precondition at the car's last spot?", body: [
                                "name": "add_precondition_schedule",
                                "preconditionMinutes": minutes(readyAt),
                            ])
                        }
                    }
                    Section("Sentry, guest, valet") {
                        commandButton("Sentry on") { ControlSend(title: "Turn sentry on?", body: ["name": "set_sentry_mode", "on": true]) }
                        commandButton("Sentry off") { ControlSend(title: "Turn sentry off?", body: ["name": "set_sentry_mode", "on": false]) }
                        commandButton("Guest mode on") { ControlSend(title: "Turn guest mode on?", body: ["name": "guest_mode", "enable": true]) }
                        commandButton("Guest mode off") { ControlSend(title: "Turn guest mode off?", body: ["name": "guest_mode", "enable": false]) }
                        SecureField("Valet code, 4 digits", text: $valetCode)
                            .keyboardType(.numberPad)
                        commandButton("Valet on", enabled: valetCode.count == 4) {
                            let code = valetCode
                            return ControlSend(title: "Turn valet on?", body: ["name": "set_valet_mode", "on": true, "password": code])
                        }
                        commandButton("Valet off") { ControlSend(title: "Turn valet off?", body: ["name": "set_valet_mode", "on": false]) }
                    }
                    Section("Speed limit mode") {
                        Stepper(value: $speedCap, in: 50...90, step: 1) {
                            Text("Cap \(Int(speedCap)) mph")
                        }
                        SecureField("Speed limit PIN, 4 digits", text: $pin)
                            .keyboardType(.numberPad)
                        commandButton("Set the cap") {
                            ControlSend(title: "Set the car's speed cap to \(Int(speedCap)) mph?", body: ["name": "speed_limit_set_limit", "limitMph": speedCap])
                        }
                        commandButton("Turn the cap on", enabled: pin.count == 4) {
                            let code = pin
                            return ControlSend(title: "Turn speed limit mode on?", body: ["name": "speed_limit_activate", "pin": code])
                        }
                        commandButton("Turn the cap off", enabled: pin.count == 4) {
                            let code = pin
                            return ControlSend(title: "Turn speed limit mode off?", body: ["name": "speed_limit_deactivate", "pin": code])
                        }
                    }
                }
                .scrollContentBackground(.hidden)
            }
            .navigationTitle("Controls")
            .toolbar {
                Button("Done") { dismiss() }
            }
            .confirmationDialog(pending?.title ?? "Send this to the car?", isPresented: Binding(
                get: { pending != nil },
                set: { if !$0 { pending = nil } }
            ), titleVisibility: .visible) {
                Button("Send") {
                    let send = pending
                    pending = nil
                    guard let send else { return }
                    Task {
                        await model.sendCommand(send.body)
                        pin = ""
                        valetCode = ""
                    }
                }
                Button("Leave it", role: .cancel) { pending = nil }
            }
        }
    }

    private func commandButton(_ title: String, enabled: Bool = true, make: @escaping () -> ControlSend) -> some View {
        Button(title) { pending = make() }
            .disabled(!enabled || model.busy)
    }

    private func minutes(_ date: Date) -> Int {
        let parts = Calendar.current.dateComponents([.hour, .minute], from: date)
        return (parts.hour ?? 0) * 60 + (parts.minute ?? 0)
    }
}

private struct ControlSend {
    var title: String
    var body: [String: Any]
}
