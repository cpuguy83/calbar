import AppKit
import Combine
import Foundation
import SwiftUI

struct Command: Decodable {
	let type: String
	let state: String?
	let tooltip: String?
	let events: [CalendarEvent]?
	let hiddenEvents: [CalendarEvent]?
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
	let meetingURL: String?
	let meetingService: String?
	let meetingID: String?
	let meetingPasscode: String?
	let meetingDialIn: String?
	let meetingPhoneID: String?
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
	@Published var selectedEvent: CalendarEvent?
	@Published var hiddenEvents: [CalendarEvent] = []
	@Published var showingHiddenEvents = false

	var onOpenURL: ((String) -> Void)?
	var onSync: (() -> Void)?
	var onHide: ((String) -> Void)?
	var onUnhide: ((String) -> Void)?
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
		if let selectedEvent {
			self.selectedEvent = events.first { $0.id == selectedEvent.id }
		}
	}

	func updateHiddenEvents(_ events: [CalendarEvent]) {
		hiddenEvents = events
	}

	func showDetails(_ event: CalendarEvent) {
		showingHiddenEvents = false
		selectedEvent = event
	}

	func hideDetails() {
		selectedEvent = nil
	}

	func showHiddenEvents() {
		selectedEvent = nil
		showingHiddenEvents = true
	}

	func hideHiddenEvents() {
		showingHiddenEvents = false
	}

	func resetNavigation() {
		selectedEvent = nil
		showingHiddenEvents = false
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

	private var timedEvents: [CalendarEvent] {
		model.filteredEvents.filter { $0.allDay != true }
	}

	private var allDayEvents: [CalendarEvent] {
		model.filteredEvents
			.filter { $0.allDay == true }
			.sorted { lhs, rhs in lhs.summary.localizedCaseInsensitiveCompare(rhs.summary) == .orderedAscending }
	}

	private var timedGroups: [EventGroup] {
		grouped(timedEvents)
	}

	private func grouped(_ events: [CalendarEvent]) -> [EventGroup] {
		var result: [EventGroup] = []
		for event in events {
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
			ZStack {
				if let event = model.selectedEvent {
					EventDetailView(
						event: event,
						onBack: { model.hideDetails() },
						onOpenURL: { model.onOpenURL?($0) },
						onHide: { uid in
							model.onHide?(uid)
							model.hideDetails()
						}
					)
					.transition(.move(edge: .trailing).combined(with: .opacity))
				} else if model.showingHiddenEvents {
					HiddenEventsView(
						events: model.hiddenEvents,
						onBack: { model.hideHiddenEvents() },
						onUnhide: { uid in
							model.onUnhide?(uid)
							model.hideHiddenEvents()
						}
					)
					.transition(.move(edge: .trailing).combined(with: .opacity))
				} else {
					listView
						.transition(.move(edge: .leading).combined(with: .opacity))
				}
			}
			.frame(maxWidth: .infinity, maxHeight: .infinity)
			Divider()
			footer
		}
		.animation(.easeInOut(duration: 0.18), value: model.selectedEvent?.id)
		.animation(.easeInOut(duration: 0.18), value: model.showingHiddenEvents)
		.frame(width: 420, height: 560)
	}

	private var listView: some View {
		VStack(spacing: 0) {
			header
			Divider()
			SearchField(text: $model.searchText, focusRequest: $model.searchFocusRequest)
				.frame(height: 28)
				.padding(.horizontal, 12)
				.padding(.vertical, 10)
			Divider()
			content
		}
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
		VStack(spacing: 0) {
			timedContent
				.frame(maxWidth: .infinity, maxHeight: .infinity)
			if !allDayEvents.isEmpty {
				Divider()
				AllDaySection(
					events: allDayEvents,
					onOpenURL: { model.onOpenURL?($0) },
					onHide: { model.onHide?($0) },
					onShowDetails: { model.showDetails($0) }
				)
			}
		}
	}

	@ViewBuilder
	private var timedContent: some View {
		if timedGroups.isEmpty && allDayEvents.isEmpty {
			ContentUnavailableView(
				model.searchText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? "No Upcoming Events" : "No Matching Events",
				systemImage: "calendar",
				description: Text(model.searchText.isEmpty ? "CalBar will show synced calendar events here." : "Try a different event, location, or calendar name.")
			)
		} else if timedGroups.isEmpty {
			ContentUnavailableView(
				"No Timed Events",
				systemImage: "weather.clear",
				description: Text("All-day events are shown below.")
			)
		} else {
			ScrollView {
				LazyVStack(alignment: .leading, spacing: 0) {
					ForEach(timedGroups) { group in
						Section {
							ForEach(group.events) { event in
								EventRow(
									event: event,
									onOpenURL: { model.onOpenURL?($0) },
									onHide: { model.onHide?($0) },
									onShowDetails: { model.showDetails(event) }
								)
							}
						} header: {
							Text(group.title.uppercased())
								.font(.caption.weight(.semibold))
								.foregroundStyle(.secondary)
								.frame(maxWidth: .infinity, alignment: .leading)
								.padding(.horizontal, 16)
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
			if !model.hiddenEvents.isEmpty {
				Button {
					model.showHiddenEvents()
				} label: {
					Label("\(model.hiddenEvents.count) hidden", systemImage: "eye.slash")
				}
				.buttonStyle(.borderless)
			}
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
	let onShowDetails: () -> Void

	var body: some View {
		HStack(alignment: .top, spacing: 12) {
			Button(action: onShowDetails) {
				rowContent
					.frame(maxWidth: .infinity, alignment: .leading)
			}
			.buttonStyle(.plain)
			if let meetingURL = event.meetingURL, !meetingURL.isEmpty {
				Button {
					onOpenURL(meetingURL)
				} label: {
					Text("Join")
				}
				.buttonStyle(.bordered)
				.controlSize(.small)
			} else {
				Image(systemName: "chevron.right")
					.font(.caption.weight(.semibold))
					.foregroundStyle(.tertiary)
					.padding(.top, 3)
			}
		}
		.padding(.horizontal, 16)
		.padding(.vertical, 10)
		.background(Color.primary.opacity(0.0001))
		.overlay(alignment: .bottom) {
			Divider().padding(.leading, 90)
		}
		.onRightClick { onHide(event.uid) }
		.accessibilityElement(children: .combine)
		.accessibilityLabel(accessibilityLabel)
	}

	private var rowContent: some View {
		HStack(alignment: .top, spacing: 12) {
			TimeColumn(event: event)
				.frame(width: 62, alignment: .trailing)
			VStack(alignment: .leading, spacing: 4) {
				HStack(alignment: .firstTextBaseline, spacing: 6) {
					Text(event.summary)
						.font(.body.weight(.medium))
						.foregroundStyle(event.stale == true ? Color.red : Color.primary)
						.lineLimit(2)
					if event.stale == true {
						Image(systemName: "exclamationmark.triangle.fill")
							.foregroundStyle(Color.red)
							.help("Stale event")
					}
				}
				Text(event.timeText)
					.font(.caption)
					.foregroundStyle(.secondary)
					.lineLimit(1)
				if let location = event.location, !location.isEmpty {
					Label(location, systemImage: "mappin.and.ellipse")
						.font(.caption)
						.foregroundStyle(.secondary)
						.lineLimit(1)
				}
				if let source = event.source, !source.isEmpty {
					Text(source)
						.font(.caption2)
						.foregroundStyle(.tertiary)
						.lineLimit(1)
				}
			}
			Spacer(minLength: 6)
		}
	}

	private var accessibilityLabel: Text {
		var parts = [event.summary, event.timeText]
		if let location = event.location, !location.isEmpty {
			parts.append(location)
		}
		return Text(parts.joined(separator: ", "))
	}
}

private struct AllDaySection: View {
	let events: [CalendarEvent]
	let onOpenURL: (String) -> Void
	let onHide: (String) -> Void
	let onShowDetails: (CalendarEvent) -> Void

	var body: some View {
		VStack(alignment: .leading, spacing: 0) {
			Text("ALL DAY")
				.font(.caption2.weight(.semibold))
				.foregroundStyle(.secondary)
				.padding(.horizontal, 16)
				.padding(.top, 8)
				.padding(.bottom, 4)
			ForEach(events) { event in
				AllDayRow(
					event: event,
					onOpenURL: onOpenURL,
					onHide: { onHide(event.uid) },
					onShowDetails: { onShowDetails(event) }
				)
			}
		}
		.frame(maxWidth: .infinity, alignment: .leading)
		.background(Color.primary.opacity(0.025))
	}
}

private struct AllDayRow: View {
	let event: CalendarEvent
	let onOpenURL: (String) -> Void
	let onHide: () -> Void
	let onShowDetails: () -> Void

	var body: some View {
		HStack(alignment: .firstTextBaseline, spacing: 8) {
			Button(action: onShowDetails) {
				VStack(alignment: .leading, spacing: 2) {
					Text(event.summary)
						.font(.callout)
						.foregroundStyle(event.stale == true ? Color.red : Color.primary)
						.lineLimit(1)
					if let metadata = allDayMetadata, !metadata.isEmpty {
						Text(metadata)
							.font(.caption2)
							.foregroundStyle(.secondary)
							.lineLimit(1)
					}
				}
				.frame(maxWidth: .infinity, alignment: .leading)
			}
			.buttonStyle(.plain)
			if let meetingURL = event.meetingURL, !meetingURL.isEmpty {
				Button {
					onOpenURL(meetingURL)
				} label: {
					Text("Join")
				}
				.buttonStyle(.bordered)
				.controlSize(.small)
			}
		}
		.padding(.horizontal, 16)
		.padding(.vertical, 7)
		.background(Color.primary.opacity(0.0001))
		.overlay(alignment: .bottom) {
			Divider().padding(.leading, 16)
		}
		.onRightClick(perform: onHide)
		.accessibilityElement(children: .combine)
		.accessibilityLabel(Text([event.summary, event.timeText].joined(separator: ", ")))
	}

	private var allDayMetadata: String? {
		let value = event.metadata ?? event.source ?? ""
		return value.isEmpty ? nil : value
	}
}

private struct HiddenEventsView: View {
	let events: [CalendarEvent]
	let onBack: () -> Void
	let onUnhide: (String) -> Void

	var body: some View {
		VStack(spacing: 0) {
			header
			Divider()
			if events.isEmpty {
				ContentUnavailableView("No Hidden Events", systemImage: "eye.slash")
					.frame(maxWidth: .infinity, maxHeight: .infinity)
			} else {
				ScrollView {
					LazyVStack(alignment: .leading, spacing: 0) {
						Text("Click an event to unhide it")
							.font(.caption)
							.foregroundStyle(.secondary)
							.padding(.horizontal, 16)
							.padding(.vertical, 10)
						ForEach(events) { event in
							HiddenEventRow(event: event) {
								onUnhide(event.uid)
							}
						}
					}
				}
			}
		}
	}

	private var header: some View {
		HStack(spacing: 8) {
			Button(action: onBack) {
				Label("Back", systemImage: "chevron.left")
					.labelStyle(.iconOnly)
			}
			.buttonStyle(.borderless)
			.help("Back")
			Text("Hidden Events")
				.font(.headline)
			Spacer()
		}
		.padding(.horizontal, 12)
		.padding(.vertical, 10)
	}
}

private struct HiddenEventRow: View {
	let event: CalendarEvent
	let onUnhide: () -> Void

	var body: some View {
		Button(action: onUnhide) {
			HStack(alignment: .center, spacing: 10) {
				VStack(alignment: .leading, spacing: 3) {
					Text(event.summary)
						.font(.body)
						.foregroundStyle(event.stale == true ? Color.red : Color.primary)
						.lineLimit(2)
					Text(event.timeText)
						.font(.caption)
						.foregroundStyle(.secondary)
						.lineLimit(1)
				}
				Spacer(minLength: 8)
				Image(systemName: "eye")
					.foregroundStyle(.secondary)
			}
			.frame(maxWidth: .infinity, alignment: .leading)
			.padding(.horizontal, 16)
			.padding(.vertical, 10)
			.background(Color.primary.opacity(0.0001))
			.overlay(alignment: .bottom) {
				Divider().padding(.leading, 16)
			}
		}
		.buttonStyle(.plain)
	}
}

private struct DetailLine: View {
	let systemImage: String
	let value: String

	var body: some View {
		HStack(alignment: .firstTextBaseline, spacing: 10) {
			Image(systemName: systemImage)
				.frame(width: 18)
				.foregroundStyle(Color.secondary)
			Text(value)
				.font(.callout)
				.foregroundStyle(.primary)
				.textSelection(.enabled)
				.fixedSize(horizontal: false, vertical: true)
		}
	}
}

private struct EventDetailView: View {
	let event: CalendarEvent
	let onBack: () -> Void
	let onOpenURL: (String) -> Void
	let onHide: (String) -> Void

	var body: some View {
		VStack(spacing: 0) {
			detailHeader
			Divider()
			ScrollView {
				VStack(alignment: .leading, spacing: 14) {
					Text(event.summary)
						.font(.title3.weight(.semibold))
						.foregroundStyle(event.stale == true ? Color.red : Color.primary)
						.fixedSize(horizontal: false, vertical: true)
					DetailLine(systemImage: "calendar", value: event.timeText)
					if let location = event.location, !location.isEmpty {
						DetailLine(systemImage: "mappin.and.ellipse", value: location)
					}
					if let service = event.meetingService, !service.isEmpty, service != event.location {
						DetailLine(systemImage: "desktopcomputer", value: service)
					}
					if let meetingID = event.meetingID, !meetingID.isEmpty {
						DetailLine(systemImage: "number", value: "Meeting ID: \(meetingID)")
					}
					if let passcode = event.meetingPasscode, !passcode.isEmpty {
						DetailLine(systemImage: "key", value: "Passcode: \(passcode)")
					}
					if let dialIn = event.meetingDialIn, !dialIn.isEmpty {
						DetailLine(systemImage: "phone", value: "Dial-in: \(dialIn)")
					}
					if let phoneID = event.meetingPhoneID, !phoneID.isEmpty {
						DetailLine(systemImage: "phone", value: "Phone conference ID: \(phoneID)")
					}
					if let organizer = event.organizer, !organizer.isEmpty {
						DetailLine(systemImage: "person", value: organizer)
					}
					if let source = event.source, !source.isEmpty {
						DetailLine(systemImage: "folder", value: source)
					}
					if let description = cleanDescription, !description.isEmpty {
						VStack(alignment: .leading, spacing: 6) {
							Text("Description")
								.font(.caption.weight(.semibold))
								.foregroundStyle(.secondary)
							Text(description)
								.font(.callout)
								.textSelection(.enabled)
								.fixedSize(horizontal: false, vertical: true)
						}
					}
					actions
				}
				.padding(16)
			}
		}
	}

	private var detailHeader: some View {
		HStack(spacing: 8) {
			Button(action: onBack) {
				Label("Back", systemImage: "chevron.left")
					.labelStyle(.iconOnly)
			}
			.buttonStyle(.borderless)
			.help("Back")
			Text("Event Details")
				.font(.headline)
			Spacer()
		}
		.padding(.horizontal, 12)
		.padding(.vertical, 10)
	}

	private var actions: some View {
		HStack(spacing: 10) {
			Spacer()
			if let meetingURL = event.meetingURL, !meetingURL.isEmpty {
				Button {
					onOpenURL(meetingURL)
				} label: {
					Label("Join", systemImage: "video")
				}
				.buttonStyle(.borderedProminent)
			}
			Button("Hide", role: .destructive) {
				onHide(event.uid)
			}
			.buttonStyle(.bordered)
			Spacer()
		}
		.controlSize(.regular)
		.padding(.top, 4)
	}

	private var cleanDescription: String? {
		guard let description = event.description else { return nil }
		return description
			.replacingOccurrences(of: "<[^>]+>", with: "", options: .regularExpression)
			.replacingOccurrences(of: "&nbsp;", with: " ")
			.trimmingCharacters(in: .whitespacesAndNewlines)
	}
}

private struct TimeColumn: View {
	let event: CalendarEvent

	var body: some View {
		VStack(alignment: .trailing, spacing: 1) {
			Text(event.timePrimaryText)
				.font(.callout.monospacedDigit().weight(.medium))
				.foregroundStyle(event.stale == true ? Color.red : Color.secondary)
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

private struct RightClickHandler: NSViewRepresentable {
	let action: () -> Void

	func makeNSView(context: Context) -> RightClickNSView {
		let view = RightClickNSView()
		view.action = action
		return view
	}

	func updateNSView(_ view: RightClickNSView, context: Context) {
		view.action = action
	}
}

private final class RightClickNSView: NSView {
	var action: (() -> Void)?

	override func hitTest(_ point: NSPoint) -> NSView? {
		guard let currentEvent = window?.currentEvent else { return nil }
		switch currentEvent.type {
		case .rightMouseDown, .rightMouseUp:
			return self
		default:
			return nil
		}
	}

	override func rightMouseUp(with event: NSEvent) {
		action?()
	}
}

private extension View {
	func onRightClick(perform action: @escaping () -> Void) -> some View {
		overlay(RightClickHandler(action: action).frame(maxWidth: .infinity, maxHeight: .infinity))
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
		model.onUnhide = { [weak self] uid in self?.send(HelperEvent(type: "unhide_event", uid: uid)) }
		model.onCopyConfigPath = { [weak self] in self?.send(HelperEvent(type: "copy_config_path")) }
		model.onQuit = { [weak self] in self?.send(HelperEvent(type: "quit")) }

		popover.behavior = .transient
		popover.contentSize = NSSize(width: 420, height: 560)
		popover.contentViewController = hostingController
		keyMonitor = NSEvent.addLocalMonitorForEvents(matching: .keyDown) { [weak self] event in
			guard event.keyCode == 53, self?.popover.isShown == true else { return event }
			if self?.model.selectedEvent != nil {
				self?.model.hideDetails()
			} else if self?.model.showingHiddenEvents == true {
				self?.model.hideHiddenEvents()
			} else {
				self?.popover.performClose(nil)
			}
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
			model.resetNavigation()
			popover.performClose(nil)
		case "toggle":
			if popover.isShown {
				model.resetNavigation()
				popover.performClose(nil)
			} else {
				showPopover()
			}
		case "search":
			showPopover()
			model.resetNavigation()
			model.requestSearchFocus()
		case "set_tray_state":
			setTrayState(command.state ?? "normal")
		case "set_tooltip":
			statusItem.button?.toolTip = command.tooltip ?? "CalBar"
		case "set_events":
			model.updateEvents(command.events ?? [])
		case "set_hidden_events":
			model.updateHiddenEvents(command.hiddenEvents ?? [])
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
		model.resetNavigation()
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
