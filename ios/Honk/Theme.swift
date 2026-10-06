import SwiftUI
import UIKit

enum HonkColor {
    static let blush = Color(red: 1.0, green: 0.86, blue: 0.90)
    static let butter = Color(red: 1.0, green: 0.95, blue: 0.76)
    static let mint = Color(red: 0.78, green: 0.93, blue: 0.86)
    static let lilac = Color(red: 0.89, green: 0.84, blue: 0.97)
    static let ink = Color(red: 0.22, green: 0.16, blue: 0.24)
    static let porch = Color(red: 1.0, green: 0.94, blue: 0.93)
}

struct HornMark: View {
    var size: CGFloat = 28

    var body: some View {
        if UIImage(systemName: "horn.fill") != nil {
            Image(systemName: "horn.fill")
                .font(.system(size: size, weight: .bold))
                .foregroundStyle(Color(red: 0.72, green: 0.22, blue: 0.42))
                .accessibilityLabel("Horn")
        } else {
            Text("HONK")
                .font(.system(size: size * 0.55, weight: .heavy, design: .rounded))
                .foregroundStyle(Color(red: 0.72, green: 0.22, blue: 0.42))
        }
    }
}

struct SpeechBubble: View {
    var text: String

    var body: some View {
        Text(text)
            .font(.body.italic())
            .foregroundStyle(HonkColor.ink)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(16)
            .background(Color.white.opacity(0.86), in: RoundedRectangle(cornerRadius: 20, style: .continuous))
            .overlay(alignment: .bottomLeading) {
                Circle()
                    .fill(Color.white.opacity(0.86))
                    .frame(width: 14, height: 14)
                    .offset(x: 22, y: 6)
            }
    }
}

struct PastelCard<Content: View>: View {
    var tint: Color
    @ViewBuilder var content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            content
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(16)
        .background(tint, in: RoundedRectangle(cornerRadius: 22, style: .continuous))
    }
}

struct HonkBackground: View {
    var body: some View {
        LinearGradient(colors: [HonkColor.porch, HonkColor.blush.opacity(0.55), HonkColor.butter.opacity(0.45)], startPoint: .top, endPoint: .bottom)
            .ignoresSafeArea()
    }
}
