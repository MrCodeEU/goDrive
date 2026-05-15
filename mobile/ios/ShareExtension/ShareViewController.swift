import UIKit
import Social
import UniformTypeIdentifiers

// Stores shared media in App Group UserDefaults using the same format
// as receive_sharing_intent's RSIShareViewController, so the Flutter plugin
// can read it via getInitialMedia() / getMediaStream().
class ShareViewController: SLComposeServiceViewController {

    private let appGroupId   = "group.com.example.godrive"
    private let bundleId     = "com.example.godrive"
    private let shareKey     = "ShareKey"

    private struct MediaItem: Encodable {
        let path: String
        let mimeType: String?
        let thumbnail: String?
        let duration: Double?
        let message: String?
        let type: String // "image" | "video" | "file"
    }

    private var collected: [MediaItem] = []
    private var pending = 0

    override func isContentValid() -> Bool { true }
    override func configurationItems() -> [Any]! { [] }

    // Auto-process as soon as the view appears — no manual Post tap needed.
    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        processAttachments()
    }

    // Required by SLComposeServiceViewController — do nothing here.
    override func didSelectPost() {}

    // MARK: - Private

    private func processAttachments() {
        let items = (extensionContext?.inputItems as? [NSExtensionItem]) ?? []
        let providers = items.flatMap { $0.attachments ?? [] }
        pending = providers.count
        guard pending > 0 else { finish(); return }
        providers.forEach(handleProvider)
    }

    private func handleProvider(_ provider: NSItemProvider) {
        if provider.hasItemConformingToTypeIdentifier(UTType.movie.identifier) {
            provider.loadFileRepresentation(forTypeIdentifier: UTType.movie.identifier) { [weak self] url, _ in
                self?.storeFile(url, mimeType: "video/mp4", type: "video")
            }
        } else if provider.hasItemConformingToTypeIdentifier(UTType.image.identifier) {
            provider.loadFileRepresentation(forTypeIdentifier: UTType.image.identifier) { [weak self] url, _ in
                let mime = url?.pathExtension.lowercased() == "png" ? "image/png" : "image/jpeg"
                self?.storeFile(url, mimeType: mime, type: "image")
            }
        } else if provider.hasItemConformingToTypeIdentifier(UTType.data.identifier) {
            provider.loadFileRepresentation(forTypeIdentifier: UTType.data.identifier) { [weak self] url, _ in
                self?.storeFile(url, mimeType: nil, type: "file")
            }
        } else {
            decrementPending()
        }
    }

    private func storeFile(_ url: URL?, mimeType: String?, type: String) {
        guard let url,
              let container = FileManager.default
                .containerURL(forSecurityApplicationGroupIdentifier: appGroupId) else {
            decrementPending(); return
        }
        let dst = container.appendingPathComponent(url.lastPathComponent)
        try? FileManager.default.removeItem(at: dst)
        do {
            try FileManager.default.copyItem(at: url, to: dst)
            let item = MediaItem(path: dst.path, mimeType: mimeType,
                                 thumbnail: nil, duration: nil, message: nil, type: type)
            DispatchQueue.main.async { self.collected.append(item) }
        } catch {}
        decrementPending()
    }

    private func decrementPending() {
        DispatchQueue.main.async {
            self.pending -= 1
            if self.pending <= 0 { self.finish() }
        }
    }

    private func finish() {
        if !collected.isEmpty,
           let data = try? JSONEncoder().encode(collected) {
            let defaults = UserDefaults(suiteName: appGroupId)
            defaults?.set(data, forKey: shareKey)
            defaults?.synchronize()
            openHostApp()
        }
        extensionContext?.completeRequest(returningItems: [], completionHandler: nil)
    }

    private func openHostApp() {
        guard let url = URL(string: "ShareMedia-\(bundleId):share") else { return }
        var responder: UIResponder? = self
        while let r = responder {
            if let app = r as? UIApplication {
                app.open(url, options: [:], completionHandler: nil)
                return
            }
            responder = r.next
        }
    }
}
