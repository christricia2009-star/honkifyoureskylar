import SwiftUI

@main
struct HonkApp: App {
    @UIApplicationDelegateAdaptor(HonkAppDelegate.self) private var delegate
    @Environment(\.scenePhase) private var scenePhase
    @State private var model = AppModel()

    var body: some Scene {
        WindowGroup {
            RootView(model: model)
                .task { await model.bootstrap() }
                .onChange(of: scenePhase) { _, phase in
                    if phase == .active {
                        PhoneTrail.shared.noteOpened()
                    }
                }
        }
    }
}
