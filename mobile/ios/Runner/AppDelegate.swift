import Flutter
import UIKit
import receive_sharing_intent

@main
@objc class AppDelegate: FlutterAppDelegate, FlutterImplicitEngineDelegate {
  override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
  ) -> Bool {
    SwiftReceiveSharingIntentPlugin.appGroupIdentifier = "group.com.example.godrive"
    SwiftReceiveSharingIntentPlugin.userDefaultsKey = "com.example.godrive-SharedDataKey"
    return super.application(application, didFinishLaunchingWithOptions: launchOptions)
  }

  override func application(
    _ app: UIApplication,
    open url: URL,
    options: [UIApplication.OpenURLOptionsKey: Any] = [:]
  ) -> Bool {
    let plugin = SwiftReceiveSharingIntentPlugin.instance
    if plugin.hasMatchingSchemePrefix(url: url) {
      return plugin.application(app, open: url, options: options)
    }
    return super.application(app, open: url, options: options)
  }

  func didInitializeImplicitFlutterEngine(_ engineBridge: FlutterImplicitEngineBridge) {
    GeneratedPluginRegistrant.register(with: engineBridge.pluginRegistry)
    registerBackgroundUploadChannel(binaryMessenger: engineBridge.applicationRegistrar.messenger())
  }

  override func application(
    _ application: UIApplication,
    handleEventsForBackgroundURLSession identifier: String,
    completionHandler: @escaping () -> Void
  ) {
    IOSBackgroundUploader.shared.setBackgroundCompletionHandler(completionHandler)
  }

  private func registerBackgroundUploadChannel(binaryMessenger: FlutterBinaryMessenger) {
    let channel = FlutterMethodChannel(
      name: "godrive/background_uploads",
      binaryMessenger: binaryMessenger
    )
    channel.setMethodCallHandler { call, result in
      switch call.method {
      case "isSupported":
        result(true)
      case "ensureNotificationPermission":
        result(true)
      case "refreshUploads":
        IOSBackgroundUploader.shared.refresh(result: result)
      case "startUpload":
        guard let args = call.arguments as? [String: Any] else {
          result(FlutterError(code: "bad_args", message: "Missing upload arguments", details: nil))
          return
        }
        IOSBackgroundUploader.shared.start(args: args, result: result)
      default:
        result(FlutterMethodNotImplemented)
      }
    }
  }
}

private final class IOSBackgroundUploader: NSObject, URLSessionDelegate, URLSessionTaskDelegate, URLSessionDataDelegate {
  static let shared = IOSBackgroundUploader()

  private let queueKey = "flutter.godrive_upload_queue"
  private let taskKey = "godrive_ios_background_tasks"
  private let sessionIdentifier = "com.example.godrive.background_uploads"
  private let staleTaskGraceSeconds: TimeInterval = 30
  private var completionHandler: (() -> Void)?
  private var responseData: [Int: Data] = [:]

  private lazy var session: URLSession = {
    let config = URLSessionConfiguration.background(withIdentifier: sessionIdentifier)
    config.sessionSendsLaunchEvents = true
    config.isDiscretionary = false
    config.httpMaximumConnectionsPerHost = 3
    return URLSession(configuration: config, delegate: self, delegateQueue: nil)
  }()

  func setBackgroundCompletionHandler(_ handler: @escaping () -> Void) {
    completionHandler = handler
    _ = session
  }

  func refresh(result: @escaping FlutterResult) {
    session.getAllTasks { tasks in
      let activeTaskIDs = Set(tasks.map { "\($0.taskIdentifier)" })
      var tracked = self.loadTasks()
      let now = Date().timeIntervalSince1970
      var changed = false

      for (taskID, dictionary) in tracked {
        if activeTaskIDs.contains(taskID) {
          continue
        }
        guard let info = TaskInfo(dictionary: dictionary) else {
          tracked.removeValue(forKey: taskID)
          changed = true
          continue
        }
        if now - info.createdAt < self.staleTaskGraceSeconds {
          continue
        }
        self.updateQueue(
          id: info.id,
          status: "error",
          error: "Background upload is no longer active. Retry the upload.",
          finalPath: nil,
          tusUrl: info.tusUrl,
          progress: nil
        )
        if info.removeBodyWhenDone {
          try? FileManager.default.removeItem(atPath: info.bodyPath)
        }
        tracked.removeValue(forKey: taskID)
        changed = true
      }

      if changed {
        self.saveTasks(tracked)
      }
      DispatchQueue.main.async {
        result(nil)
      }
    }
  }

  func start(args: [String: Any], result: @escaping FlutterResult) {
    guard let request = UploadRequest(args: args) else {
      result(FlutterError(code: "bad_args", message: "Invalid upload arguments", details: nil))
      return
    }

    updateQueue(id: request.id, status: "background", error: nil, finalPath: nil, tusUrl: request.tusUrl, progress: nil)

    DispatchQueue.global(qos: .utility).async {
      do {
        let tusURL = try self.ensureUploadURL(request: request)
        request.tusUrl = tusURL
        self.updateQueue(id: request.id, status: "background", error: nil, finalPath: nil, tusUrl: tusURL, progress: nil)

        let offset = try self.fetchOffset(request: request, tusURL: tusURL)
        let activeTusURL = request.tusUrl ?? tusURL
        if offset >= request.fileSize {
          self.updateQueue(
            id: request.id,
            status: "done",
            error: nil,
            finalPath: "\(request.targetPath)/\(request.filename)",
            tusUrl: activeTusURL,
            progress: 1.0
          )
          DispatchQueue.main.async { result(nil) }
          return
        }

        try self.schedulePatch(request: request, tusURL: activeTusURL, offset: offset)
        DispatchQueue.main.async { result(nil) }
      } catch {
        self.updateQueue(
          id: request.id,
          status: "error",
          error: error.localizedDescription,
          finalPath: nil,
          tusUrl: request.tusUrl,
          progress: nil
        )
        DispatchQueue.main.async {
          result(FlutterError(code: "upload_failed", message: error.localizedDescription, details: nil))
        }
      }
    }
  }

  private func ensureUploadURL(request: UploadRequest) throws -> String {
    if let tusURL = request.tusUrl, !tusURL.isEmpty {
      return tusURL
    }

    var components = URLComponents(string: request.baseUrl.trimmedTrailingSlash + "/api/tus")
    components?.queryItems = [URLQueryItem(name: "path", value: request.targetPath)]
    guard let url = components?.url else { throw UploadError.message("Invalid server URL") }

    var urlRequest = URLRequest(url: url)
    urlRequest.httpMethod = "POST"
    urlRequest.timeoutInterval = 30
    urlRequest.setValue("Bearer \(request.token)", forHTTPHeaderField: "Authorization")
    urlRequest.setValue("1.0.0", forHTTPHeaderField: "Tus-Resumable")
    urlRequest.setValue("\(request.fileSize)", forHTTPHeaderField: "Upload-Length")
    urlRequest.setValue("filename \(Data(request.filename.utf8).base64EncodedString())", forHTTPHeaderField: "Upload-Metadata")

    let (_, response) = try synchronousDataTask(urlRequest)
    guard response.statusCode == 201 else {
      throw UploadError.message("Upload create failed (\(response.statusCode))")
    }
    guard let location = header(response, "Location"), !location.isEmpty else {
      throw UploadError.message("Upload endpoint did not return Location")
    }
    return location
  }

  private func fetchOffset(request: UploadRequest, tusURL: String) throws -> Int64 {
    guard let url = URL(string: resolveURL(baseUrl: request.baseUrl, tusURL: tusURL)) else {
      throw UploadError.message("Invalid TUS URL")
    }
    var urlRequest = URLRequest(url: url)
    urlRequest.httpMethod = "HEAD"
    urlRequest.timeoutInterval = 30
    urlRequest.setValue("Bearer \(request.token)", forHTTPHeaderField: "Authorization")
    urlRequest.setValue("1.0.0", forHTTPHeaderField: "Tus-Resumable")

    let (_, response) = try synchronousDataTask(urlRequest)
    if response.statusCode == 404 {
      request.tusUrl = nil
      let replacement = try ensureUploadURL(request: request)
      request.tusUrl = replacement
      updateQueue(id: request.id, status: "background", error: nil, finalPath: nil, tusUrl: replacement, progress: nil)
      return 0
    }
    guard response.statusCode == 204 else {
      throw UploadError.message("HEAD failed (\(response.statusCode))")
    }
    request.tusUrl = tusURL
    return Int64(header(response, "Upload-Offset") ?? "0") ?? 0
  }

  private func schedulePatch(request: UploadRequest, tusURL: String, offset: Int64) throws {
    guard let url = URL(string: resolveURL(baseUrl: request.baseUrl, tusURL: tusURL)) else {
      throw UploadError.message("Invalid TUS URL")
    }

    let source = URL(fileURLWithPath: request.filePath)
    guard FileManager.default.isReadableFile(atPath: source.path) else {
      throw UploadError.message("File is no longer available on this device")
    }

    let bodyURL = try uploadBodyURL(source: source, request: request, offset: offset)
    var urlRequest = URLRequest(url: url)
    urlRequest.httpMethod = "PATCH"
    urlRequest.setValue("Bearer \(request.token)", forHTTPHeaderField: "Authorization")
    urlRequest.setValue("application/offset+octet-stream", forHTTPHeaderField: "Content-Type")
    urlRequest.setValue("1.0.0", forHTTPHeaderField: "Tus-Resumable")
    urlRequest.setValue("\(offset)", forHTTPHeaderField: "Upload-Offset")

    let task = session.uploadTask(with: urlRequest, fromFile: bodyURL)
    storeTask(
      taskID: task.taskIdentifier,
      info: TaskInfo(
        id: request.id,
        filename: request.filename,
        targetPath: request.targetPath,
        fileSize: request.fileSize,
        offset: offset,
        tusUrl: tusURL,
        bodyPath: bodyURL.path,
        removeBodyWhenDone: bodyURL.path != source.path,
        createdAt: Date().timeIntervalSince1970
      )
    )
    task.resume()
  }

  private func uploadBodyURL(source: URL, request: UploadRequest, offset: Int64) throws -> URL {
    if offset <= 0 {
      return source
    }

    let destination = FileManager.default.temporaryDirectory
      .appendingPathComponent("godrive-\(request.id)-\(offset).upload")
    FileManager.default.createFile(atPath: destination.path, contents: nil)

    let input = try FileHandle(forReadingFrom: source)
    defer { try? input.close() }
    let output = try FileHandle(forWritingTo: destination)
    defer { try? output.close() }

    try input.seek(toOffset: UInt64(offset))
    while true {
      let data = input.readData(ofLength: 256 * 1024)
      if data.isEmpty { break }
      try output.write(contentsOf: data)
    }
    return destination
  }

  func urlSession(
    _ session: URLSession,
    task: URLSessionTask,
    didSendBodyData bytesSent: Int64,
    totalBytesSent: Int64,
    totalBytesExpectedToSend: Int64
  ) {
    guard let info = taskInfo(taskID: task.taskIdentifier) else { return }
    let sent = min(info.fileSize, info.offset + totalBytesSent)
    let progress = Double(sent) / Double(max(info.fileSize, 1))
    updateQueue(id: info.id, status: "background", error: nil, finalPath: nil, tusUrl: info.tusUrl, progress: progress)
  }

  func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
    var existing = responseData[dataTask.taskIdentifier] ?? Data()
    existing.append(data)
    responseData[dataTask.taskIdentifier] = existing
  }

  func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
    guard let info = taskInfo(taskID: task.taskIdentifier) else { return }
    defer {
      removeTask(taskID: task.taskIdentifier)
      responseData.removeValue(forKey: task.taskIdentifier)
      if info.removeBodyWhenDone {
        try? FileManager.default.removeItem(atPath: info.bodyPath)
      }
    }

    if let error = error {
      updateQueue(id: info.id, status: "error", error: error.localizedDescription, finalPath: nil, tusUrl: info.tusUrl, progress: nil)
      return
    }

    guard let response = task.response as? HTTPURLResponse else {
      updateQueue(id: info.id, status: "error", error: "Upload response missing", finalPath: nil, tusUrl: info.tusUrl, progress: nil)
      return
    }

    guard (200...299).contains(response.statusCode) else {
      let body = responseData[task.taskIdentifier].flatMap { String(data: $0, encoding: .utf8) }
      updateQueue(
        id: info.id,
        status: "error",
        error: body?.isEmpty == false ? body : "Upload chunk failed (\(response.statusCode))",
        finalPath: nil,
        tusUrl: info.tusUrl,
        progress: nil
      )
      return
    }

    let finalPath = header(response, "Upload-Final-Path") ?? "\(info.targetPath)/\(info.filename)"
    updateQueue(id: info.id, status: "done", error: nil, finalPath: finalPath, tusUrl: info.tusUrl, progress: 1.0)
  }

  func urlSessionDidFinishEvents(forBackgroundURLSession session: URLSession) {
    DispatchQueue.main.async {
      self.completionHandler?()
      self.completionHandler = nil
    }
  }

  private func synchronousDataTask(_ request: URLRequest) throws -> (Data, HTTPURLResponse) {
    let semaphore = DispatchSemaphore(value: 0)
    var result: Result<(Data, HTTPURLResponse), Error>!
    URLSession.shared.dataTask(with: request) { data, response, error in
      if let error = error {
        result = .failure(error)
      } else if let response = response as? HTTPURLResponse {
        result = .success((data ?? Data(), response))
      } else {
        result = .failure(UploadError.message("HTTP response missing"))
      }
      semaphore.signal()
    }.resume()
    semaphore.wait()
    return try result.get()
  }

  private func updateQueue(
    id: String,
    status: String,
    error: String?,
    finalPath: String?,
    tusUrl: String?,
    progress: Double?
  ) {
    let defaults = UserDefaults.standard
    let data = defaults.string(forKey: queueKey)?.data(using: .utf8) ?? Data("[]".utf8)
    guard var array = (try? JSONSerialization.jsonObject(with: data)) as? [[String: Any]] else { return }
    guard let index = array.firstIndex(where: { ($0["id"] as? String) == id }) else { return }

    array[index]["status"] = status
    if let progress {
      array[index]["progress"] = progress
    } else if status == "done" {
      array[index]["progress"] = 1.0
    }
    array[index]["error"] = error
    array[index]["final_path"] = finalPath
    array[index]["tus_url"] = tusUrl

    if let encoded = try? JSONSerialization.data(withJSONObject: array),
       let raw = String(data: encoded, encoding: .utf8) {
      defaults.set(raw, forKey: queueKey)
    }
  }

  private func storeTask(taskID: Int, info: TaskInfo) {
    var all = loadTasks()
    all["\(taskID)"] = info.dictionary
    saveTasks(all)
  }

  private func taskInfo(taskID: Int) -> TaskInfo? {
    TaskInfo(dictionary: loadTasks()["\(taskID)"])
  }

  private func removeTask(taskID: Int) {
    var all = loadTasks()
    all.removeValue(forKey: "\(taskID)")
    saveTasks(all)
  }

  private func loadTasks() -> [String: [String: Any]] {
    guard let data = UserDefaults.standard.data(forKey: taskKey),
          let all = try? JSONSerialization.jsonObject(with: data) as? [String: [String: Any]] else {
      return [:]
    }
    return all
  }

  private func saveTasks(_ tasks: [String: [String: Any]]) {
    let data = try? JSONSerialization.data(withJSONObject: tasks)
    UserDefaults.standard.set(data, forKey: taskKey)
  }

  private func resolveURL(baseUrl: String, tusURL: String) -> String {
    if tusURL.hasPrefix("http://") || tusURL.hasPrefix("https://") {
      return tusURL
    }
    return baseUrl.trimmedTrailingSlash + tusURL
  }

  private func header(_ response: HTTPURLResponse, _ name: String) -> String? {
    for (key, value) in response.allHeaderFields {
      if String(describing: key).caseInsensitiveCompare(name) == .orderedSame {
        return String(describing: value)
      }
    }
    return nil
  }

  private final class UploadRequest {
    let id: String
    let filePath: String
    let filename: String
    let fileSize: Int64
    let targetPath: String
    var tusUrl: String?
    let baseUrl: String
    let token: String

    init?(args: [String: Any]) {
      guard let id = args["id"] as? String,
            let filePath = args["filePath"] as? String,
            let filename = args["filename"] as? String,
            let fileSize = args["fileSize"] as? NSNumber,
            let targetPath = args["targetPath"] as? String,
            let baseUrl = args["baseUrl"] as? String,
            let token = args["token"] as? String else {
        return nil
      }
      self.id = id
      self.filePath = filePath
      self.filename = filename
      self.fileSize = fileSize.int64Value
      self.targetPath = targetPath
      self.tusUrl = args["tusUrl"] as? String
      self.baseUrl = baseUrl
      self.token = token
    }
  }

  private struct TaskInfo {
    let id: String
    let filename: String
    let targetPath: String
    let fileSize: Int64
    let offset: Int64
    let tusUrl: String
    let bodyPath: String
    let removeBodyWhenDone: Bool
    let createdAt: TimeInterval

    var dictionary: [String: Any] {
      [
        "id": id,
        "filename": filename,
        "targetPath": targetPath,
        "fileSize": fileSize,
        "offset": offset,
        "tusUrl": tusUrl,
        "bodyPath": bodyPath,
        "removeBodyWhenDone": removeBodyWhenDone,
        "createdAt": createdAt
      ]
    }

    init?(dictionary: [String: Any]?) {
      guard let dictionary,
            let id = dictionary["id"] as? String,
            let filename = dictionary["filename"] as? String,
            let targetPath = dictionary["targetPath"] as? String,
            let fileSize = dictionary["fileSize"] as? NSNumber,
            let offset = dictionary["offset"] as? NSNumber,
            let tusUrl = dictionary["tusUrl"] as? String,
            let bodyPath = dictionary["bodyPath"] as? String,
            let removeBodyWhenDone = dictionary["removeBodyWhenDone"] as? Bool else {
        return nil
      }
      let createdAt = (dictionary["createdAt"] as? NSNumber)?.doubleValue ?? 0
      self.id = id
      self.filename = filename
      self.targetPath = targetPath
      self.fileSize = fileSize.int64Value
      self.offset = offset.int64Value
      self.tusUrl = tusUrl
      self.bodyPath = bodyPath
      self.removeBodyWhenDone = removeBodyWhenDone
      self.createdAt = createdAt
    }

    init(
      id: String,
      filename: String,
      targetPath: String,
      fileSize: Int64,
      offset: Int64,
      tusUrl: String,
      bodyPath: String,
      removeBodyWhenDone: Bool,
      createdAt: TimeInterval
    ) {
      self.id = id
      self.filename = filename
      self.targetPath = targetPath
      self.fileSize = fileSize
      self.offset = offset
      self.tusUrl = tusUrl
      self.bodyPath = bodyPath
      self.removeBodyWhenDone = removeBodyWhenDone
      self.createdAt = createdAt
    }
  }

  private enum UploadError: LocalizedError {
    case message(String)

    var errorDescription: String? {
      switch self {
      case .message(let message):
        return message
      }
    }
  }
}

private extension String {
  var trimmedTrailingSlash: String {
    var value = self
    while value.hasSuffix("/") {
      value.removeLast()
    }
    return value
  }
}
