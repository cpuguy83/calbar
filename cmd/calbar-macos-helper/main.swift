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
    let section: String?
    let timeText: String
    let timePrimary: String?
    let timeSecondary: String?
    let metadata: String?
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
    enum ButtonStyle {
        case standard
        case primary
        case quiet
    }

    private let handler: () -> Void

    init(title: String, style: ButtonStyle = .standard, handler: @escaping () -> Void) {
        self.handler = handler
        super.init(frame: .zero)
        self.title = title
        self.target = self
        self.action = #selector(runHandler)
        self.controlSize = .small
        self.font = NSFont.systemFont(ofSize: 12, weight: style == .primary ? .semibold : .regular)

        switch style {
        case .standard:
            bezelStyle = .rounded
        case .primary:
            bezelStyle = .rounded
            contentTintColor = .controlAccentColor
        case .quiet:
            isBordered = false
            contentTintColor = .secondaryLabelColor
        }
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

        let titleStack = NSStackView()
        titleStack.orientation = .vertical
        titleStack.alignment = .leading
        titleStack.spacing = 2

        statusLabel.textColor = .secondaryLabelColor
        statusLabel.font = NSFont.systemFont(ofSize: 12)

        titleStack.addArrangedSubview(title)
        titleStack.addArrangedSubview(statusLabel)

        let spacer = NSView()
        spacer.setContentHuggingPriority(.defaultLow, for: .horizontal)

        let syncButton = ActionButton(title: "Sync") { [weak self] in self?.onSync?() }
        syncButton.image = NSImage(systemSymbolName: "arrow.clockwise", accessibilityDescription: "Sync")
        syncButton.imagePosition = .imageLeading

        header.addArrangedSubview(titleStack)
        header.addArrangedSubview(spacer)
        header.addArrangedSubview(syncButton)

        searchField.placeholderString = "Search events"
        searchField.delegate = self

        let scrollView = NSScrollView()
        scrollView.hasVerticalScroller = true
        scrollView.borderType = .noBorder
        scrollView.translatesAutoresizingMaskIntoConstraints = false

        eventsStack.orientation = .vertical
        eventsStack.alignment = .leading
        eventsStack.spacing = 0
        eventsStack.edgeInsets = NSEdgeInsets(top: 2, left: 0, bottom: 2, right: 0)
        eventsStack.translatesAutoresizingMaskIntoConstraints = false

        scrollView.documentView = eventsStack

        let footer = NSStackView()
        footer.orientation = .horizontal
        footer.alignment = .centerY
        footer.spacing = 8

        let footerSpacer = NSView()
        footerSpacer.setContentHuggingPriority(.defaultLow, for: .horizontal)
        let copyButton = ActionButton(title: "Copy Config", style: .quiet) { [weak self] in self?.onCopyConfigPath?() }
        let quitButton = ActionButton(title: "Quit", style: .quiet) { [weak self] in self?.onQuit?() }
        footer.addArrangedSubview(footerSpacer)
        footer.addArrangedSubview(copyButton)
        footer.addArrangedSubview(quitButton)

        content.addArrangedSubview(header)
        content.addArrangedSubview(searchField)
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
            scrollView.widthAnchor.constraint(equalTo: content.widthAnchor, constant: -28),
            scrollView.heightAnchor.constraint(equalToConstant: 380),
            footer.widthAnchor.constraint(equalTo: content.widthAnchor, constant: -28),
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
            empty.font = NSFont.systemFont(ofSize: 13)
            eventsStack.addArrangedSubview(empty)
            return
        }

        var previousSection: String?
        for event in visible {
            let section = sectionTitle(for: event)
            if section != previousSection {
                let header = makeSectionHeader(section)
                eventsStack.addArrangedSubview(header)
                header.widthAnchor.constraint(equalTo: eventsStack.widthAnchor, constant: -2).isActive = true
                previousSection = section
            }
            let row = makeEventRow(event)
            eventsStack.addArrangedSubview(row)
            row.widthAnchor.constraint(equalTo: eventsStack.widthAnchor, constant: -2).isActive = true
        }
    }

    private func sectionTitle(for event: CalendarEvent) -> String {
        if let section = event.section, !section.isEmpty {
            return section
        }
        return "Later"
    }

    private func makeSectionHeader(_ title: String) -> NSView {
        let label = NSTextField(labelWithString: title.uppercased())
        label.font = NSFont.systemFont(ofSize: 11, weight: .semibold)
        label.textColor = .secondaryLabelColor

        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.edgeInsets = NSEdgeInsets(top: 10, left: 0, bottom: 4, right: 0)
        stack.translatesAutoresizingMaskIntoConstraints = false
        stack.addArrangedSubview(label)
        return stack
    }

    private func makeEventRow(_ event: CalendarEvent) -> NSView {
        let row = NSStackView()
        row.orientation = .horizontal
        row.alignment = .top
        row.spacing = 10
        row.edgeInsets = NSEdgeInsets(top: 8, left: 0, bottom: 8, right: 0)

        let timeColumn = makeTimeColumn(event)

        let details = NSStackView()
        details.orientation = .vertical
        details.alignment = .leading
        details.spacing = 3

        let titleLine = NSStackView()
        titleLine.orientation = .horizontal
        titleLine.alignment = .firstBaseline
        titleLine.spacing = 7

        let title = NSTextField(labelWithString: event.summary)
        title.font = NSFont.systemFont(ofSize: 13, weight: .semibold)
        title.lineBreakMode = .byTruncatingTail
        title.maximumNumberOfLines = 2

        titleLine.addArrangedSubview(title)
        if event.stale == true {
            let stale = NSTextField(labelWithString: "Stale")
            stale.font = NSFont.systemFont(ofSize: 11, weight: .medium)
            stale.textColor = .systemRed
            titleLine.addArrangedSubview(stale)
        }

        let time = NSTextField(labelWithString: event.timeText)
        time.font = NSFont.systemFont(ofSize: 12)
        time.textColor = .secondaryLabelColor
        time.lineBreakMode = .byTruncatingTail

        details.addArrangedSubview(titleLine)
        details.addArrangedSubview(time)

        if let location = event.location, !location.isEmpty {
            let locationLabel = NSTextField(labelWithString: location)
            locationLabel.font = NSFont.systemFont(ofSize: 12)
            locationLabel.textColor = .tertiaryLabelColor
            locationLabel.lineBreakMode = .byTruncatingTail
            details.addArrangedSubview(locationLabel)
        }

        let actions = NSStackView()
        actions.orientation = .horizontal
        actions.spacing = 6

        if let meetingURL = event.meetingURL, !meetingURL.isEmpty {
            actions.addArrangedSubview(ActionButton(title: "Join") { [weak self] in self?.onOpenURL?(meetingURL) })
        }
        actions.addArrangedSubview(ActionButton(title: "Hide", style: .quiet) { [weak self] in self?.onHide?(event.uid) })
        details.addArrangedSubview(actions)

        row.addArrangedSubview(timeColumn)
        row.addArrangedSubview(details)
        row.translatesAutoresizingMaskIntoConstraints = false
        timeColumn.widthAnchor.constraint(equalToConstant: 72).isActive = true
        return row
    }

    private func makeTimeColumn(_ event: CalendarEvent) -> NSView {
        let stack = NSStackView()
        stack.orientation = .vertical
        stack.alignment = .trailing
        stack.spacing = 1

        let pieces = timePieces(for: event)
        let primary = NSTextField(labelWithString: pieces.primary)
        primary.font = NSFont.monospacedDigitSystemFont(ofSize: 12, weight: .medium)
        primary.textColor = .secondaryLabelColor
        primary.alignment = .right

        let secondary = NSTextField(labelWithString: pieces.secondary)
        secondary.font = NSFont.monospacedDigitSystemFont(ofSize: 11, weight: .regular)
        secondary.textColor = .tertiaryLabelColor
        secondary.alignment = .right

        stack.addArrangedSubview(primary)
        if !pieces.secondary.isEmpty {
            stack.addArrangedSubview(secondary)
        }
        return stack
    }

    private func timePieces(for event: CalendarEvent) -> (primary: String, secondary: String) {
        if event.allDay == true {
            return ("All day", "")
        }
        if let primary = event.timePrimary, !primary.isEmpty {
            return (primary, event.timeSecondary ?? "")
        }
        return (event.timeText, "")
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
        popover.contentSize = NSSize(width: 380, height: 490)
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
