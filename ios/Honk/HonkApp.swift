import SwiftUI

@main
struct HonkApp: App {
    @UIApplicationDelegateAdaptor(HonkAppDelegate.self) private var delegate
    @State private var model = AppModel()

    var body: some Scene {
        WindowGroup {
            RootView(model: model)
                .task { await model.bootstrap() }
        }
    }
}
