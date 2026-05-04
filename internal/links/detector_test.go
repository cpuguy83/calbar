package links

import "testing"

func TestDetectFromEvent_TeamsMeetURL(t *testing.T) {
	url := "https://teams.microsoft.com/meet/22792173431568?p=d4qiBuwhjR0xQOLil6"
	if got := DetectFromEvent("", "Join: "+url, ""); got != url {
		t.Fatalf("unexpected URL: got %q want %q", got, url)
	}
	if got := Service(url); got != "Teams" {
		t.Fatalf("unexpected service: got %q want Teams", got)
	}
}
