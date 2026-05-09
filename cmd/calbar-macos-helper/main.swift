import AppKit
import Foundation

struct Command: Decodable {
    let type: String
    let state: String?
    let tooltip: String?
    let events: [CalendarEvent]?
    let loading: Bool?
    let stale: Bool?
    let errors: [String]?
}

struct CalendarEvent: Codable {
    let uid: String
    let summary: String
    let timeText: String
    let location: String?
    let source: String?
    let meetingURL: String?
    let allDay: Bool?
    let stale: Bool?
}

struct HelperEvent: Encodable {
    let type: String
    let url: String?
    let uid: String?

    init(type: String, url: String? = nil, uid: String? = nil) {
        self.type = type
        self.url = url
        self.uid = uid
    }
}

final class ActionButton: NSButton {
    private let handler: () -> Void

    init(title: String, handler: @escaping () -> Void) {
        self.handler = handler
        super.init(frame: .zero)
        self.title = title
        self.bezelStyle = .rounded
        self.target = self
        self.action = #selector(runHandler)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    @objc private func runHandler() {
        handler()
    }
}

final class CalendarViewController: NSViewController, NSSearchFieldDelegate {
    var onOpenURL: ((String) -> Void)?
    var onSync: (() -> Void)?
    var onHide: ((String) -> Void)?
    var onCopyConfigPath: (() -> Void)?
    var onQuit: (() -> Void)?

    private var events: [CalendarEvent] = []
    private var loading = false
    private var stale = false
    private var errors: [String] = []

    private let statusLabel = NSTextField(labelWithString: "")
    private let searchField = NSSearchField()
    private let eventsStack = NSStackView()

    override func loadView() {
        let root = NSVisualEffectView()
        root.material = .popover
        root.blendingMode = .withinWindow
        root.state = .active

        let content = NSStackView()
        content.orientation = .vertical
        content.alignment = .leading
        content.spacing = 10
        content.edgeInsets = NSEdgeInsets(top: 14, left: 14, bottom: 14, right: 14)
        content.translatesAutoresizingMaskIntoConstraints = false

        let header = NSStackView()
        header.orientation = .horizontal
        header.alignment = .centerY
        header.spacing = 8

        let title = NSTextField(labelWithString: "CalBar")
        title.font = NSFont.systemFont(ofSize: 18, weight: .semibold)

        let spacer = NSView()
        spacer.setContentHuggingPriority(.defaultLow, for: .horizontal)

        let syncButton = ActionButton(title: "Sync") { [weak self] in self?.onSync?() }

        header.addArrangedSubview(title)
        header.addArrangedSubview(spacer)
        header.addArrangedSubview(syncButton)

        searchField.placeholderString = "Search events"
        searchField.delegate = self

        statusLabel.textColor = .secondaryLabelColor
        statusLabel.font = NSFont.systemFont(ofSize: 12)

        let scrollView = NSScrollView()
        scrollView.hasVerticalScroller = true
        scrollView.borderType = .noBorder
        scrollView.translatesAutoresizingMaskIntoConstraints = false

        eventsStack.orientation = .vertical
        eventsStack.alignment = .leading
        eventsStack.spacing = 8
        eventsStack.edgeInsets = NSEdgeInsets(top: 2, left: 0, bottom: 2, right: 0)
        eventsStack.translatesAutoresizingMaskIntoConstraints = false

        scrollView.documentView = eventsStack

        let footer = NSStackView()
        footer.orientation = .horizontal
        footer.spacing = 8

        let copyButton = ActionButton(title: "Copy Config Path") { [weak self] in self?.onCopyConfigPath?() }
        let quitButton = ActionButton(title: "Quit") { [weak self] in self?.onQuit?() }
        footer.addArrangedSubview(copyButton)
        footer.addArrangedSubview(quitButton)

        content.addArrangedSubview(header)
        content.addArrangedSubview(searchField)
        content.addArrangedSubview(statusLabel)
        content.addArrangedSubview(scrollView)
        content.addArrangedSubview(footer)

        root.addSubview(content)
        NSLayoutConstraint.activate([
            content.leadingAnchor.constraint(equalTo: root.leadingAnchor),
            content.trailingAnchor.constraint(equalTo: root.trailingAnchor),
            content.topAnchor.constraint(equalTo: root.topAnchor),
            content.bottomAnchor.constraint(equalTo: root.bottomAnchor),
            header.widthAnchor.constraint(equalTo: content.widthAnchor, constant: -28),
            searchField.widthAnchor.constraint(equalTo: content.widthAnchor, constant: -28),
            statusLabel.widthAnchor.constraint(equalTo: content.widthAnchor, constant: -28),
            scrollView.widthAnchor.constraint(equalTo: content.widthAnchor, constant: -28),
            scrollView.heightAnchor.constraint(equalToConstant: 360),
            eventsStack.widthAnchor.constraint(equalTo: scrollView.contentView.widthAnchor)
        ])

        view = root
        rebuild()
    }

    func updateEvents(_ events: [CalendarEvent]) {
        self.events = events
        if isViewLoaded { rebuild() }
    }

    func updateLoading(_ loading: Bool) {
        self.loading = loading
        if isViewLoaded { rebuildStatus() }
    }

    func updateStale(_ stale: Bool) {
        self.stale = stale
        if isViewLoaded { rebuildStatus() }
    }

    func updateErrors(_ errors: [String]) {
        self.errors = errors
        if isViewLoaded { rebuildStatus() }
    }

    func focusSearch() {
        view.window?.makeFirstResponder(searchField)
    }

    func controlTextDidChange(_ obj: Notification) {
        rebuildEvents()
    }

    private func rebuild() {
        rebuildStatus()
        rebuildEvents()
    }

    private func rebuildStatus() {
        if loading {
            statusLabel.stringValue = "Syncing calendars..."
        } else if !errors.isEmpty {
            statusLabel.stringValue = errors.joined(separator: "  ")
            statusLabel.textColor = .systemRed
            return
        } else if stale {
            statusLabel.stringValue = "Calendar data may be stale"
        } else {
            statusLabel.stringValue = "Upcoming events"
        }
        statusLabel.textColor = .secondaryLabelColor
    }

    private func rebuildEvents() {
        for subview in eventsStack.arrangedSubviews {
            eventsStack.removeArrangedSubview(subview)
            subview.removeFromSuperview()
        }

        let query = searchField.stringValue.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        let visible = events.filter { event in
            if query.isEmpty { return true }
            return event.summary.lowercased().contains(query)
                || (event.location ?? "").lowercased().contains(query)
                || (event.source ?? "").lowercased().contains(query)
        }

        if visible.isEmpty {
            let empty = NSTextField(labelWithString: query.isEmpty ? "No upcoming events" : "No matching events")
            empty.textColor = .secondaryLabelColor
            eventsStack.addArrangedSubview(empty)
            return
        }

        for event in visible {
            let row = makeEventRow(event)
            eventsStack.addArrangedSubview(row)
            row.widthAnchor.constraint(equalTo: eventsStack.widthAnchor, constant: -2).isActive = true
        }
    }

    private func makeEventRow(_ event: CalendarEvent) -> NSView {
        let row = NSStackView()
        row.orientation = .vertical
        row.alignment = .leading
        row.spacing = 4
        row.edgeInsets = NSEdgeInsets(top: 8, left: 8, bottom: 8, right: 8)

        let titleLine = NSStackView()
        titleLine.orientation = .horizontal
        titleLine.alignment = .firstBaseline
        titleLine.spacing = 6

        let title = NSTextField(labelWithString: event.summary)
        title.font = NSFont.systemFont(ofSize: 13, weight: .semibold)
        title.lineBreakMode = .byTruncatingTail

        if event.stale == true {
            let stale = NSTextField(labelWithString: "Stale")
            stale.font = NSFont.systemFont(ofSize: 11, weight: .medium)
            stale.textColor = .systemRed
            titleLine.addArrangedSubview(stale)
        }

        titleLine.addArrangedSubview(title)

        let time = NSTextField(labelWithString: event.timeText)
        time.font = NSFont.systemFont(ofSize: 12)
        time.textColor = .secondaryLabelColor

        row.addArrangedSubview(titleLine)
        row.addArrangedSubview(time)

        if let location = event.location, !location.isEmpty {
            let locationLabel = NSTextField(labelWithString: location)
            locationLabel.font = NSFont.systemFont(ofSize: 12)
            locationLabel.textColor = .tertiaryLabelColor
            locationLabel.lineBreakMode = .byTruncatingTail
            row.addArrangedSubview(locationLabel)
        }

        let actions = NSStackView()
        actions.orientation = .horizontal
        actions.spacing = 6

        if let meetingURL = event.meetingURL, !meetingURL.isEmpty {
            actions.addArrangedSubview(ActionButton(title: "Join") { [weak self] in self?.onOpenURL?(meetingURL) })
        }
        actions.addArrangedSubview(ActionButton(title: "Hide") { [weak self] in self?.onHide?(event.uid) })
        row.addArrangedSubview(actions)

        let box = NSBox()
        box.boxType = .custom
        box.borderType = .lineBorder
        box.borderColor = NSColor.separatorColor
        box.cornerRadius = 8
        box.contentViewMargins = NSSize(width: 0, height: 0)
        box.translatesAutoresizingMaskIntoConstraints = false
        row.translatesAutoresizingMaskIntoConstraints = false
        box.addSubview(row)
        NSLayoutConstraint.activate([
            row.leadingAnchor.constraint(equalTo: box.leadingAnchor),
            row.trailingAnchor.constraint(equalTo: box.trailingAnchor),
            row.topAnchor.constraint(equalTo: box.topAnchor),
            row.bottomAnchor.constraint(equalTo: box.bottomAnchor)
        ])
        return box
    }
}

final class HelperApp: NSObject, NSApplicationDelegate {
    private let statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
    private let popover = NSPopover()
    private let viewController = CalendarViewController()
    private let decoder = JSONDecoder()
    private let encoder = JSONEncoder()

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)

        viewController.onOpenURL = { [weak self] url in self?.send(HelperEvent(type: "open_url", url: url)) }
        viewController.onSync = { [weak self] in self?.send(HelperEvent(type: "sync")) }
        viewController.onHide = { [weak self] uid in self?.send(HelperEvent(type: "hide_event", uid: uid)) }
        viewController.onCopyConfigPath = { [weak self] in self?.send(HelperEvent(type: "copy_config_path")) }
        viewController.onQuit = { [weak self] in self?.send(HelperEvent(type: "quit")) }

        popover.behavior = .transient
        popover.contentSize = NSSize(width: 380, height: 520)
        popover.contentViewController = viewController

        if let button = statusItem.button {
            button.target = self
            button.action = #selector(statusItemClicked)
            button.sendAction(on: [.leftMouseUp, .rightMouseUp])
            button.toolTip = "CalBar"
        }
        setTrayState("normal")
        readCommands()
    }

    @objc private func statusItemClicked() {
        if NSApp.currentEvent?.type == .rightMouseUp {
            showContextMenu()
            return
        }
        send(HelperEvent(type: "activate"))
    }

    private func showContextMenu() {
        let menu = NSMenu()
        menu.addItem(menuItem(title: "Show Calendar", action: #selector(showCalendarFromMenu)))
        menu.addItem(menuItem(title: "Sync", action: #selector(syncFromMenu)))
        menu.addItem(menuItem(title: "Copy Config Path", action: #selector(copyConfigPathFromMenu)))
        menu.addItem(.separator())
        menu.addItem(menuItem(title: "Quit", action: #selector(quitFromMenu)))

        statusItem.menu = menu
        statusItem.button?.performClick(nil)
        statusItem.menu = nil
    }

    private func menuItem(title: String, action: Selector) -> NSMenuItem {
        let item = NSMenuItem(title: title, action: action, keyEquivalent: "")
        item.target = self
        return item
    }

    @objc private func showCalendarFromMenu() { showPopover() }
    @objc private func syncFromMenu() { send(HelperEvent(type: "sync")) }
    @objc private func copyConfigPathFromMenu() { send(HelperEvent(type: "copy_config_path")) }
    @objc private func quitFromMenu() { send(HelperEvent(type: "quit")) }

    private func readCommands() {
        DispatchQueue.global(qos: .userInitiated).async { [weak self] in
            while let line = readLine() {
                guard let self = self, let data = line.data(using: .utf8) else { continue }
                do {
                    let command = try self.decoder.decode(Command.self, from: data)
                    DispatchQueue.main.async { self.handle(command) }
                } catch {
                    fputs("calbar-macos-helper: invalid command: \(error)\n", stderr)
                }
            }
            DispatchQueue.main.async { NSApp.terminate(nil) }
        }
    }

    private func handle(_ command: Command) {
        switch command.type {
        case "show":
            showPopover()
        case "hide":
            popover.performClose(nil)
        case "toggle":
            popover.isShown ? popover.performClose(nil) : showPopover()
        case "search":
            showPopover()
            viewController.focusSearch()
        case "set_tray_state":
            setTrayState(command.state ?? "normal")
        case "set_tooltip":
            statusItem.button?.toolTip = command.tooltip ?? "CalBar"
        case "set_events":
            viewController.updateEvents(command.events ?? [])
        case "set_loading":
            viewController.updateLoading(command.loading ?? false)
        case "set_stale":
            viewController.updateStale(command.stale ?? false)
        case "set_sync_errors":
            viewController.updateErrors(command.errors ?? [])
        default:
            fputs("calbar-macos-helper: unknown command \(command.type)\n", stderr)
        }
    }

    private func showPopover() {
        guard let button = statusItem.button else { return }
        if popover.isShown { return }
        popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
        NSApp.activate(ignoringOtherApps: true)
    }

    private func setTrayState(_ state: String) {
        let symbol: String
        let color: NSColor?
        switch state {
        case "imminent":
            symbol = "clock.badge.exclamationmark"
            color = .systemOrange
        case "stale":
            symbol = "exclamationmark.triangle"
            color = .systemRed
        default:
            symbol = "calendar"
            color = nil
        }

        let image = NSImage(systemSymbolName: symbol, accessibilityDescription: "CalBar")
            ?? NSImage(systemSymbolName: "calendar", accessibilityDescription: "CalBar")
        image?.isTemplate = true
        statusItem.button?.image = image
        statusItem.button?.contentTintColor = color
    }

    private func send(_ event: HelperEvent) {
        do {
            let data = try encoder.encode(event)
            if let line = String(data: data, encoding: .utf8) {
                FileHandle.standardOutput.write((line + "\n").data(using: .utf8)!)
            }
        } catch {
            fputs("calbar-macos-helper: encode event: \(error)\n", stderr)
        }
    }
}

let app = NSApplication.shared
let delegate = HelperApp()
app.delegate = delegate
app.run()
