package main

var controlCommandMethods = map[string]string{
	"show":   "Show",
	"hide":   "Hide",
	"toggle": "Toggle",
	"search": "Search",
	"sync":   "Sync",
	"quit":   "Quit",
}

var controlCommandNames = []string{
	"show",
	"hide",
	"toggle",
	"search",
	"sync",
	"quit",
}

var controlCommandDescriptions = map[string]string{
	"show":   "Show the configured CalBar UI",
	"hide":   "Hide the configured CalBar UI",
	"toggle": "Toggle the configured CalBar UI",
	"search": "Show the configured CalBar UI and focus search when supported",
	"sync":   "Trigger a calendar sync",
	"quit":   "Quit the running CalBar instance",
}
