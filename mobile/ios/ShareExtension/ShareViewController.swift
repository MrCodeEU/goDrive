import UIKit
import Social
import MobileCoreServices
import Photos
import UniformTypeIdentifiers

// Passes shared media paths to the main app via the App Group shared container.
// The main app reads these via receive_sharing_intent on launch/resume.
class ShareViewController: UIViewController {
    private let appGroupId = "group.com.example.godrive"
    private let userDefaultsKey = "com.example.godrive-SharedDataKey"
    private var mediaItems: [[String: Any]] = []
    private var pendingCount = 0

    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        guard let items = extensionContext?.inputItems as? [NSExtensionItem] else {
            close(); return
        }
        let attachments = items.flatMap { $0.attachments ?? [] }
        pendingCount = attachments.count
        guard pendingCount > 0 else { close(); return }
        for provider in attachments {
            handleProvider(provider)
        }
    }

    private func handleProvider(_ provider: NSItemProvider) {
        if provider.hasItemConformingToTypeIdentifier(UTType.movie.identifier) {
            provider.loadFileRepresentation(forTypeIdentifier: UTType.movie.identifier) { [weak self] url, error in
                guard let self, let url else { self?.decrementPending(); return }
                self.copyToGroup(url: url, mimeType: "video/mp4", type: 1)
            }
        } else if provider.hasItemConformingToTypeIdentifier(UTType.image.identifier) {
            provider.loadFileRepresentation(forTypeIdentifier: UTType.image.identifier) { [weak self] url, error in
                guard let self, let url else { self?.decrementPending(); return }
                self.copyToGroup(url: url, mimeType: "image/jpeg", type: 0)
            }
        } else if provider.hasItemConformingToTypeIdentifier(UTType.data.identifier) {
            provider.loadFileRepresentation(forTypeIdentifier: UTType.data.identifier) { [weak self] url, error in
                guard let self, let url else { self?.decrementPending(); return }
                self.copyToGroup(url: url, mimeType: "application/octet-stream", type: 2)
            }
        } else {
            decrementPending()
        }
    }

    private func copyToGroup(url: URL, mimeType: String, type: Int) {
        guard let container = FileManager.default
            .containerURL(forSecurityApplicationGroupIdentifier: appGroupId)?
            .appendingPathComponent("shared", isDirectory: true) else {
            decrementPending(); return
        }
        try? FileManager.default.createDirectory(at: container, withIntermediateDirectories: true)
        let dest = container.appendingPathComponent(url.lastPathComponent)
        try? FileManager.default.removeItem(at: dest)
        do {
            try FileManager.default.copyItem(at: url, to: dest)
            DispatchQueue.main.async {
                self.mediaItems.append([
                    "path": dest.path,
                    "mimeType": mimeType,
                    "type": type,
                    "thumbnail": NSNull(),
                    "duration": NSNull(),
                ])
                self.decrementPending()
            }
        } catch {
            DispatchQueue.main.async { self.decrementPending() }
        }
    }

    private func decrementPending() {
        pendingCount -= 1
        if pendingCount <= 0 { finish() }
    }

    private func finish() {
        if !mediaItems.isEmpty,
           let data = try? JSONSerialization.data(withJSONObject: mediaItems),
           let json = String(data: data, encoding: .utf8) {
            let defaults = UserDefaults(suiteName: appGroupId)
            defaults?.set(json, forKey: userDefaultsKey)
            defaults?.synchronize()
            // Open main app
            let url = URL(string: "godrive://share")!
            var responder: UIResponder? = self
            while let r = responder {
                if let app = r as? UIApplication {
                    app.open(url)
                    break
                }
                responder = r.next
            }
        }
        close()
    }

    private func close() {
        extensionContext?.completeRequest(returningItems: [], completionHandler: nil)
    }
}
