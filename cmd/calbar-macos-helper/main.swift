import AppKit
import Combine
import Foundation
import SwiftUI

struct Command: Decodable {
	let type: String
	let state: String?
	let tooltip: String?
	let events: [CalendarEvent]?
	let loading: Bool?
	let stale: Bool?
	let errors: [String]?
}

struct CalendarEvent: Codable, Identifiable {
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

	var id: String { uid }
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

final class CalendarModel: ObservableObject {
	@Published var events: [CalendarEvent] = []
	@Published var loading = false
	@Published var stale = false
	@Published var errors: [String] = []
	@Published var searchText = ""
	@Published var searchFocusRequest = 0

	var onOpenURL: ((String) -> Void)?
	var onSync: (() -> Void)?
	var onHide: ((String) -> Void)?
	var onCopyConfigPath: (() -> Void)?
	var onQuit: (() -> Void)?

	var statusText: String {
		if loading {
			return "Syncing calendars..."
		}
		if !errors.isEmpty {
			return errors.joined(separator: "  ")
		}
		if stale {
			return "Calendar data may be stale"
		}
		return "Upcoming events"
	}

	var filteredEvents: [CalendarEvent] {
		let query = searchText.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
		guard !query.isEmpty else { return events }
		return events.filter { event in
			event.summary.lowercased().contains(query)
				|| (event.location ?? "").lowercased().contains(query)
				|| (event.source ?? "").lowercased().contains(query)
		}
	}

	func requestSearchFocus() {
		searchFocusRequest += 1
	}

	func updateEvents(_ events: [CalendarEvent]) {
		self.events = events
	}

	func updateLoading(_ loading: Bool) {
		self.loading = loading
	}

	func updateStale(_ stale: Bool) {
		self.stale = stale
	}

	func updateErrors(_ errors: [String]) {
		self.errors = errors
	}
}

private struct EventGroup: Identifiable {
	let id: String
	let title: String
	var events: [CalendarEvent]
}

private struct CalendarPopoverView: View {
	@ObservedObject var model: CalendarModel

	private var groups: [EventGroup] {
		var result: [EventGroup] = []
		for event in model.filteredEvents {
			let title = event.sectionTitle
			if let last = result.indices.last, result[last].title == title {
				result[last].events.append(event)
			} else {
				result.append(EventGroup(id: "\(result.count)-\(title)", title: title, events: [event]))
			}
		}
		return result
	}

	var body: some View {
		VStack(spacing: 0) {
			header
			Divider()
			SearchField(text: $model.searchText, focusRequest: $model.searchFocusRequest)
				.frame(height: 28)
				.padding(.horizontal, 12)
				.padding(.vertical, 10)
			Divider()
			content
			Divider()
			footer
		}
		.frame(width: 400, height: 520)
	}

	private var header: some View {
		HStack(spacing: 10) {
			Image(systemName: "calendar")
				.font(.title3)
				.symbolRenderingMode(.hierarchical)
				.accessibilityHidden(true)
			VStack(alignment: .leading, spacing: 2) {
				Text("CalBar")
					.font(.headline)
				Text(model.statusText)
					.font(.caption)
					.foregroundStyle(model.errors.isEmpty ? Color.secondary : Color.red)
					.lineLimit(1)
			}
			Spacer(minLength: 12)
			if model.loading {
				ProgressView()
					.controlSize(.small)
			}
			Button {
				model.onSync?()
			} label: {
				Label("Sync", systemImage: "arrow.clockwise")
					.labelStyle(.iconOnly)
			}
			.buttonStyle(.borderless)
			.help("Sync calendars")
			.keyboardShortcut("r", modifiers: .command)
		}
		.padding(.horizontal, 12)
		.padding(.vertical, 10)
	}

	@ViewBuilder
	private var content: some View {
		if groups.isEmpty {
			ContentUnavailableView(
				model.searchText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? "No Upcoming Events" : "No Matching Events",
				systemImage: "calendar",
				description: Text(model.searchText.isEmpty ? "CalBar will show synced calendar events here." : "Try a different event, location, or calendar name.")
			)
			.frame(maxWidth: .infinity, maxHeight: .infinity)
		} else {
			ScrollView {
				LazyVStack(alignment: .leading, spacing: 0) {
					ForEach(groups) { group in
						Section {
							ForEach(group.events) { event in
								EventRow(
									event: event,
									onOpenURL: { model.onOpenURL?($0) },
									onHide: { model.onHide?($0) }
								)
							}
						} header: {
							Text(group.title.uppercased())
								.font(.caption.weight(.semibold))
								.foregroundStyle(.secondary)
								.frame(maxWidth: .infinity, alignment: .leading)
								.padding(.horizontal, 12)
								.padding(.top, 12)
								.padding(.bottom, 4)
						}
					}
				}
				.padding(.bottom, 8)
			}
		}
	}

	private var footer: some View {
		HStack(spacing: 12) {
			Button {
				model.onCopyConfigPath?()
			} label: {
				Label("Copy Config Path", systemImage: "doc.on.doc")
			}
			.buttonStyle(.borderless)
			Spacer()
			Button {
				model.onQuit?()
			} label: {
				Label("Quit", systemImage: "power")
			}
			.buttonStyle(.borderless)
		}
		.controlSize(.small)
		.padding(.horizontal, 12)
		.padding(.vertical, 8)
	}
}

private struct EventRow: View {
	let event: CalendarEvent
	let onOpenURL: (String) -> Void
	let onHide: (String) -> Void

	var body: some View {
		HStack(alignment: .top, spacing: 12) {
			TimeColumn(event: event)
				.frame(width: 72, alignment: .trailing)
			VStack(alignment: .leading, spacing: 4) {
				HStack(alignment: .firstTextBaseline, spacing: 6) {
					Text(event.summary)
						.font(.body.weight(.semibold))
						.lineLimit(2)
					if event.stale == true {
						Image(systemName: "exclamationmark.triangle.fill")
							.foregroundStyle(.red)
							.help("Stale event")
					}
				}
				Text(event.timeText)
					.font(.callout)
					.foregroundStyle(.secondary)
					.lineLimit(1)
				if let location = event.location, !location.isEmpty {
					Text(location)
						.font(.caption)
						.foregroundStyle(.secondary)
						.lineLimit(1)
				}
				if let metadata = event.metadata, !metadata.isEmpty {
					Text(metadata)
						.font(.caption2)
						.foregroundStyle(.tertiary)
						.lineLimit(1)
				}
				HStack(spacing: 8) {
					if let meetingURL = event.meetingURL, !meetingURL.isEmpty {
						Button {
							onOpenURL(meetingURL)
						} label: {
							Label("Join", systemImage: "video")
						}
						.buttonStyle(.bordered)
					}
					Button("Hide") {
						onHide(event.uid)
					}
					.buttonStyle(.borderless)
					.foregroundStyle(.secondary)
				}
				.controlSize(.small)
				.padding(.top, 2)
			}
		}
		.padding(.horizontal, 12)
		.padding(.vertical, 8)
		.contentShape(Rectangle())
		.accessibilityElement(children: .combine)
		.accessibilityLabel(accessibilityLabel)
	}

	private var accessibilityLabel: Text {
		var parts = [event.summary, event.timeText]
		if let location = event.location, !location.isEmpty {
			parts.append(location)
		}
		return Text(parts.joined(separator: ", "))
	}
}

private struct TimeColumn: View {
	let event: CalendarEvent

	var body: some View {
		VStack(alignment: .trailing, spacing: 1) {
			Text(event.timePrimaryText)
				.font(.callout.monospacedDigit().weight(.medium))
				.foregroundStyle(.secondary)
				.lineLimit(1)
			if !event.timeSecondaryText.isEmpty {
				Text(event.timeSecondaryText)
					.font(.caption.monospacedDigit())
					.foregroundStyle(.tertiary)
					.lineLimit(1)
			}
		}
	}
}

private struct SearchField: NSViewRepresentable {
	@Binding var text: String
	@Binding var focusRequest: Int

	func makeCoordinator() -> Coordinator {
		Coordinator(text: $text)
	}

	func makeNSView(context: Context) -> NSSearchField {
		let field = NSSearchField()
		field.placeholderString = "Search events"
		field.delegate = context.coordinator
		field.sendsSearchStringImmediately = true
		field.controlSize = .regular
		return field
	}

	func updateNSView(_ field: NSSearchField, context: Context) {
		if field.stringValue != text {
			field.stringValue = text
		}
		guard context.coordinator.focusRequest != focusRequest else { return }
		context.coordinator.focusRequest = focusRequest
		DispatchQueue.main.async {
			field.window?.makeFirstResponder(field)
		}
	}

	final class Coordinator: NSObject, NSSearchFieldDelegate {
		@Binding var text: String
		var focusRequest = 0

		init(text: Binding<String>) {
			_text = text
		}

		func controlTextDidChange(_ obj: Notification) {
			guard let field = obj.object as? NSSearchField else { return }
			text = field.stringValue
		}
	}
}

private extension CalendarEvent {
	var sectionTitle: String {
		if let section, !section.isEmpty {
			return section
		}
		return "Later"
	}

	var timePrimaryText: String {
		if allDay == true {
			return "All day"
		}
		if let timePrimary, !timePrimary.isEmpty {
			return timePrimary
		}
		return timeText
	}

	var timeSecondaryText: String {
		if allDay == true {
			return ""
		}
		return timeSecondary ?? ""
	}
}

final class HelperApp: NSObject, NSApplicationDelegate {
	private let statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
	private let popover = NSPopover()
	private let model = CalendarModel()
	private let decoder = JSONDecoder()
	private let encoder = JSONEncoder()
	private var keyMonitor: Any?
	private lazy var hostingController = NSHostingController(rootView: CalendarPopoverView(model: model))

	func applicationDidFinishLaunching(_ notification: Notification) {
		NSApp.setActivationPolicy(.accessory)

		model.onOpenURL = { [weak self] url in self?.send(HelperEvent(type: "open_url", url: url)) }
		model.onSync = { [weak self] in self?.send(HelperEvent(type: "sync")) }
		model.onHide = { [weak self] uid in self?.send(HelperEvent(type: "hide_event", uid: uid)) }
		model.onCopyConfigPath = { [weak self] in self?.send(HelperEvent(type: "copy_config_path")) }
		model.onQuit = { [weak self] in self?.send(HelperEvent(type: "quit")) }

		popover.behavior = .transient
		popover.contentSize = NSSize(width: 400, height: 520)
		popover.contentViewController = hostingController
		keyMonitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] event in
			guard event.keyCode == 53, self?.popover.isShown == true else { return event }
			self?.popover.performClose(nil)
			return nil
		}

		if let button = statusItem.button {
			button.target = self
			button.action = #selector(statusItemClicked)
			button.sendAction(on: [.leftMouseUp, .rightMouseUp])
			button.toolTip = "CalBar"
			button.imagePosition = .imageOnly
			button.setAccessibilityLabel("CalBar")
		}
		setTrayState("normal")
		readCommands()
	}

	func applicationWillTerminate(_ notification: Notification) {
		if let keyMonitor {
			NSEvent.removeMonitor(keyMonitor)
		}
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
				guard let self, let data = line.data(using: .utf8) else { continue }
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
			model.requestSearchFocus()
		case "set_tray_state":
			setTrayState(command.state ?? "normal")
		case "set_tooltip":
			statusItem.button?.toolTip = command.tooltip ?? "CalBar"
		case "set_events":
			model.updateEvents(command.events ?? [])
		case "set_loading":
			model.updateLoading(command.loading ?? false)
		case "set_stale":
			model.updateStale(command.stale ?? false)
		case "set_sync_errors":
			model.updateErrors(command.errors ?? [])
		default:
			fputs("calbar-macos-helper: unknown command \(command.type)\n", stderr)
		}
	}

	private func showPopover() {
		guard let button = statusItem.button else { return }
		if popover.isShown { return }
		popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
		NSApp.activate()
	}

	private func setTrayState(_ state: String) {
		let symbol: String
		switch state {
		case "imminent":
			symbol = "clock.badge.exclamationmark"
		case "stale":
			symbol = "exclamationmark.triangle"
		default:
			symbol = "calendar"
		}

		let image = NSImage(systemSymbolName: symbol, accessibilityDescription: "CalBar")
			?? NSImage(systemSymbolName: "calendar", accessibilityDescription: "CalBar")
		image?.isTemplate = true
		statusItem.button?.image = image
		statusItem.button?.contentTintColor = nil
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
