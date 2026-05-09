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
	let description: String?
	let section: String?
	let timeText: String
	let timePrimary: String?
	let timeSecondary: String?
	let metadata: String?
	let location: String?
	let organizer: String?
	let source: String?
	let eventURL: String?
	let meetingURL: String?
	let meetingService: String?
	let meetingID: String?
	let meetingPasscode: String?
	let meetingDialIn: String?
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
				|| (event.description ?? "").lowercased().contains(query)
				|| (event.location ?? "").lowercased().contains(query)
				|| (event.organizer ?? "").lowercased().contains(query)
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
	@State private var expandedEventID: String?

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
		.frame(width: 420, height: 560)
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
				LazyVStack(alignment: .leading, spacing: 8) {
					ForEach(groups) { group in
						Section {
							ForEach(group.events) { event in
								EventRow(
									event: event,
									isExpanded: expandedEventID == event.id,
									onOpenURL: { model.onOpenURL?($0) },
									onHide: { model.onHide?($0) },
									onToggleDetails: {
										withAnimation(.easeInOut(duration: 0.16)) {
											expandedEventID = expandedEventID == event.id ? nil : event.id
										}
									}
								)
							}
						} header: {
							Text(group.title.uppercased())
								.font(.caption.weight(.semibold))
								.foregroundStyle(.secondary)
								.frame(maxWidth: .infinity, alignment: .leading)
								.padding(.horizontal, 14)
								.padding(.top, 14)
								.padding(.bottom, 2)
						}
					}
				}
				.padding(.top, 4)
				.padding(.bottom, 10)
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
	let isExpanded: Bool
	let onOpenURL: (String) -> Void
	let onHide: (String) -> Void
	let onToggleDetails: () -> Void

	var body: some View {
		VStack(alignment: .leading, spacing: 0) {
			HStack(alignment: .top, spacing: 12) {
				TimeColumn(event: event, accentColor: accentColor)
					.frame(width: 74, alignment: .trailing)
				VStack(alignment: .leading, spacing: 6) {
					HStack(alignment: .firstTextBaseline, spacing: 6) {
						Text(event.summary)
							.font(.body.weight(.semibold))
							.foregroundStyle(.primary)
							.lineLimit(2)
						if event.stale == true {
							Image(systemName: "exclamationmark.triangle.fill")
								.foregroundStyle(Color.red)
								.help("Stale event")
						}
					}
					Text(event.timeText)
						.font(.callout)
						.foregroundStyle(Color.primary.opacity(0.78))
						.lineLimit(1)
					if let location = event.location, !location.isEmpty {
						Label(location, systemImage: "mappin.and.ellipse")
							.font(.caption)
							.foregroundStyle(.secondary)
							.lineLimit(1)
					}
					HStack(spacing: 6) {
						if let source = event.source, !source.isEmpty {
							EventBadge(text: source, systemImage: "calendar", color: accentColor)
						}
						if event.meetingURL?.isEmpty == false {
							EventBadge(text: "Meeting", systemImage: "video.fill", color: accentColor)
						}
					}
				}
				Spacer(minLength: 6)
				VStack(alignment: .trailing, spacing: 6) {
					if let meetingURL = event.meetingURL, !meetingURL.isEmpty {
						Button {
							onOpenURL(meetingURL)
						} label: {
							Label("Join", systemImage: "video")
						}
						.buttonStyle(.bordered)
					}
					Button {
						onToggleDetails()
					} label: {
						Label(isExpanded ? "Less" : "Details", systemImage: isExpanded ? "chevron.up" : "info.circle")
							.labelStyle(.iconOnly)
					}
					.buttonStyle(.borderless)
					.foregroundStyle(.secondary)
					.help(isExpanded ? "Hide details" : "Show details")
				}
				.controlSize(.small)
			}
			.padding(12)
			if isExpanded {
				Divider()
					.padding(.leading, 98)
					.padding(.trailing, 12)
				details
					.transition(.opacity.combined(with: .move(edge: .top)))
			}
		}
		.background(cardFill)
		.clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
		.overlay {
			RoundedRectangle(cornerRadius: 14, style: .continuous)
				.strokeBorder(cardStroke, lineWidth: event.stale == true ? 1.5 : 1)
		}
		.overlay(alignment: .leading) {
			Capsule()
				.fill(accentColor)
				.frame(width: 4)
				.padding(.vertical, 11)
				.padding(.leading, 8)
		}
		.shadow(color: Color.black.opacity(0.08), radius: 6, x: 0, y: 2)
		.padding(.horizontal, 10)
		.padding(.vertical, 2)
		.accessibilityElement(children: .combine)
		.accessibilityLabel(accessibilityLabel)
	}

	private var details: some View {
		VStack(alignment: .leading, spacing: 8) {
			DetailLine(systemImage: "clock", title: "When", value: event.timeText)
			if let location = event.location, !location.isEmpty {
				DetailLine(systemImage: "mappin.and.ellipse", title: "Where", value: location)
			}
			if let source = event.source, !source.isEmpty {
				DetailLine(systemImage: "calendar", title: "Calendar", value: source)
			}
			if let organizer = event.organizer, !organizer.isEmpty {
				DetailLine(systemImage: "person", title: "Organizer", value: organizer)
			}
			if let service = event.meetingService, !service.isEmpty {
				DetailLine(systemImage: "video", title: "Meeting", value: service)
			}
			if let meetingID = event.meetingID, !meetingID.isEmpty {
				DetailLine(systemImage: "number", title: "Meeting ID", value: meetingID)
			}
			if let passcode = event.meetingPasscode, !passcode.isEmpty {
				DetailLine(systemImage: "key", title: "Passcode", value: passcode)
			}
			if let dialIn = event.meetingDialIn, !dialIn.isEmpty {
				DetailLine(systemImage: "phone", title: "Dial-in", value: dialIn)
			}
			if let metadata = event.metadata, !metadata.isEmpty, metadata != event.source {
				DetailLine(systemImage: "info.circle", title: "Info", value: metadata)
			}
			if let description = event.description?.trimmingCharacters(in: .whitespacesAndNewlines), !description.isEmpty {
				Text(description)
					.font(.caption)
					.foregroundStyle(.primary)
					.lineLimit(5)
					.frame(maxWidth: .infinity, alignment: .leading)
					.padding(.top, 2)
			}
			HStack(spacing: 8) {
				if let meetingURL = event.meetingURL, !meetingURL.isEmpty {
					Button {
						onOpenURL(meetingURL)
					} label: {
						Label("Join Meeting", systemImage: "video.fill")
					}
					.buttonStyle(.borderedProminent)
				}
				if let eventURL = event.eventURL, !eventURL.isEmpty, eventURL != event.meetingURL {
					Button {
						onOpenURL(eventURL)
					} label: {
						Label("Open Event", systemImage: "arrow.up.right.square")
					}
					.buttonStyle(.bordered)
				}
				Button("Hide Event", role: .destructive) {
					onHide(event.uid)
				}
				.buttonStyle(.borderless)
			}
			.controlSize(.small)
			.padding(.top, 2)
		}
		.padding(.leading, 98)
		.padding(.trailing, 12)
		.padding(.vertical, 10)
	}

	private var cardFill: Color {
		if event.stale == true {
			return Color.red.opacity(0.08)
		}
		return Color(nsColor: .textBackgroundColor).opacity(0.92)
	}

	private var cardStroke: Color {
		if event.stale == true {
			return Color.red.opacity(0.5)
		}
		return Color.primary.opacity(0.18)
	}

	private var accentColor: Color {
		let key = event.source?.isEmpty == false ? event.source! : event.summary
		var hash = 0
		for scalar in key.unicodeScalars {
			hash = (hash &* 31) &+ Int(scalar.value)
		}
		return Self.accentColors[abs(hash % Self.accentColors.count)]
	}

	private static let accentColors: [Color] = [
		.blue,
		.purple,
		.teal,
		.orange,
		.green,
		.pink,
		.indigo,
	]

	private var accessibilityLabel: Text {
		var parts = [event.summary, event.timeText]
		if let location = event.location, !location.isEmpty {
			parts.append(location)
		}
		return Text(parts.joined(separator: ", "))
	}
}

private struct EventBadge: View {
	let text: String
	let systemImage: String
	let color: Color

	var body: some View {
		Label(text, systemImage: systemImage)
			.font(.caption2.weight(.medium))
			.foregroundStyle(color)
			.lineLimit(1)
			.padding(.horizontal, 7)
			.padding(.vertical, 3)
			.background(color.opacity(0.12), in: Capsule())
	}
}

private struct DetailLine: View {
	let systemImage: String
	let title: String
	let value: String

	var body: some View {
		HStack(alignment: .firstTextBaseline, spacing: 8) {
			Image(systemName: systemImage)
				.frame(width: 14)
				.foregroundStyle(.secondary)
			Text(title)
				.font(.caption.weight(.semibold))
				.foregroundStyle(.secondary)
				.frame(width: 66, alignment: .leading)
			Text(value)
				.font(.caption)
				.foregroundStyle(.primary)
				.lineLimit(2)
		}
		.fixedSize(horizontal: false, vertical: true)
	}
}

private struct TimeColumn: View {
	let event: CalendarEvent
	let accentColor: Color

	var body: some View {
		VStack(alignment: .trailing, spacing: 1) {
			Text(event.timePrimaryText)
				.font(.callout.monospacedDigit().weight(.semibold))
				.foregroundStyle(accentColor)
				.lineLimit(1)
			if !event.timeSecondaryText.isEmpty {
				Text(event.timeSecondaryText)
					.font(.caption.monospacedDigit())
					.foregroundStyle(.secondary)
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
		popover.contentSize = NSSize(width: 420, height: 560)
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
