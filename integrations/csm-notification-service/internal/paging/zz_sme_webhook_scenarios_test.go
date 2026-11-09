package paging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/eventbus"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/events"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/notifications"
)

// throttle spaces every webhook request and records each status, so the space keeps the order and the run reports failures.
type throttle struct {
	mu       sync.Mutex
	next     http.RoundTripper
	statuses []int
}

func (th *throttle) RoundTrip(r *http.Request) (*http.Response, error) {
	th.mu.Lock()
	defer th.mu.Unlock()
	if os.Getenv("SAMPLE_FAST") == "" {
		time.Sleep(1500 * time.Millisecond)
	}
	resp, err := th.next.RoundTrip(r)
	if err == nil {
		th.statuses = append(th.statuses, resp.StatusCode)
	}
	return resp, err
}

func rostered(name, team, shift, tier string) onDutyAssignment {
	a := onDutyFor(name, name+"@example.com", team)
	a.Engineer.Name = name
	a.ShiftCode = shift
	if tier != "" {
		a.Tier = &tier
	}
	return a
}

func TestZZSMEWebhookScenarios(t *testing.T) {
	hook := os.Getenv("SAMPLE_WEBHOOK")
	if hook == "" {
		t.Skip()
	}
	th := &throttle{next: http.DefaultTransport}
	http.DefaultTransport = th
	defer func() { http.DefaultTransport = th.next }()
	chat := notifications.NewGoogleChatClient(notifications.GoogleChatConfig{AudienceSpaces: []notifications.GoogleChatAudienceSpace{
		{Audience: "Special Ops", WebhookURL: hook}, {Audience: "SRE", WebhookURL: hook},
	}})
	ctx := context.Background()

	say := func(text string) {
		body, _ := json.Marshal(map[string]string{"text": text})
		resp, err := http.Post(hook, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	engine := func(onDuty []onDutyAssignment) *Engine {
		r := rotaReader(true)
		r.onDuty = append([]onDutyAssignment{
			rostered("Ruwan Silva", "apollo", "SRE_TZ1_L1", ""),
			rostered("Ishara Bandara", "iaas", "SRE_IAAS_DAY", "L1"),
		}, onDuty...)
		sme := smeOn
		sme.UnansweredChat = true
		e := smeEngine(r, newMemStore(), &fakeNotes{}, &fakeChat{}, &fakeChat{}, sme)
		e.notifiers = []notifier{chatNotifier{chat: chat, links: fakeLinks{}, audience: "SRE"}}
		e.smeNotifiers = []notifier{chatNotifier{chat: chat, links: fakeLinks{}, audience: "Special Ops"}}
		e.smeSharedRoom = &chatRoom{chat: chat, room: "Special Ops"}
		e.cfg.Ladder.SMEHandoffChat = true
		e.sharedRoom = &chatRoom{chat: chat, room: "SRE"}
		return e
	}
	alert := func(n int, previous, team string, at time.Time) events.IncidentSpecialOpsAlertPayload {
		p := handoff(previous, team, team, at)
		p.IncidentID = fmt.Sprintf("sme-scenario-%d-20261026", n)
		p.Number = fmt.Sprintf("INC00260%02d", n)
		p.Subject = "Login failures for multiple tenants after the gateway rollout"
		p.Priority, p.ServiceName, p.State = "P1", "Asgardeo Identity Server", "IN_PROGRESS"
		p.ChangedBy = "Amali Jayasuriya"
		return p
	}
	// open raises the incident on its SRE team and places the SRE L1, so the "Incident Reported" card is always the first card.
	open := func(e *Engine, p events.IncidentSpecialOpsAlertPayload) {
		raw, _ := json.Marshal(events.IncidentCreatedPayload{
			Title: p.Subject, ShortDescription: p.Subject, Number: p.Number, Priority: p.Priority,
			Team: p.PreviousAssignmentGroupName, ReportedAt: testClock.Add(-10 * time.Minute).Format(time.RFC3339),
		})
		body, _ := json.Marshal(events.Envelope{Type: events.TypeIncidentCreated, EntityID: p.IncidentID, Payload: raw})
		if err := e.Handle(ctx, eventbus.Record{Value: body}); err != nil {
			t.Fatal(err)
		}
		if err := e.Tick(ctx, testClock.Add(-10*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	escalate := func(e *Engine, p events.IncidentSpecialOpsAlertPayload) {
		if err := e.HandleSpecialOpsAlert(ctx, p.IncidentID, p); err != nil {
			t.Fatal(err)
		}
	}
	tick := func(e *Engine, after ...time.Duration) {
		for _, d := range after {
			if err := e.Tick(ctx, testClock.Add(d)); err != nil {
				t.Fatal(err)
			}
		}
	}
	assign := func(e *Engine, incidentID, who string, at time.Time) {
		raw, _ := json.Marshal(events.IncidentAssignedPayload{AssigneeID: who, AssigneeName: who, AssignedOn: at.Format(time.RFC3339)})
		body, _ := json.Marshal(events.Envelope{Type: events.TypeIncidentAssigned, EntityID: incidentID, Payload: raw})
		if err := e.Handle(ctx, eventbus.Record{Value: body}); err != nil {
			t.Fatal(err)
		}
	}
	full := []onDutyAssignment{
		rostered("Kasun Perera", "asgardeo", "SME_ASG_DAY", "L1"),
		rostered("Nimal Fernando", "asgardeo", "SME_ASG_DAY", "L2"),
		rostered("Dilini Wickramasinghe", "asgardeo", "SME_ASG_DAY", "L3"),
	}
	ladder := []time.Duration{0, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 20 * time.Minute, 30 * time.Minute}

	first := "the SRE \"Incident Reported\" card with its L1 reply (Ruwan Silva)"
	scenario := func(n int, what, expect string) {
		say(fmt.Sprintf("*Testing SME escalation (%d/10): %s.* Expect: %s", n, what, expect))
	}

	scenario(1, "L1 on call, nobody answers", first+", the SRE handoff message (paging), the Incident Escalated card naming Kasun Perera, L1, L2 and L3 replies, then the unanswered closing message.")
	e := engine(full)
	p := alert(1, "Apollo", "asgardeo", testClock)
	open(e, p)
	escalate(e, p)
	tick(e, ladder...)

	scenario(2, "L1 on call, assigned after L1", first+", the handoff message, the card naming Kasun Perera, the L1 reply, then a stopped reply. No L2 or L3.")
	e = engine(full)
	p = alert(2, "Apollo", "asgardeo", testClock)
	open(e, p)
	escalate(e, p)
	tick(e, 0)
	assign(e, p.IncidentID, "Kasun Perera", testClock.Add(2*time.Minute))
	tick(e, ladder[1:]...)

	scenario(3, "nobody rostered on any tier", first+", the handoff message (not paging), then the card with \"Nobody rostered\". No SME replies.")
	e = engine(nil)
	p = alert(3, "Apollo", "bijira", testClock)
	open(e, p)
	escalate(e, p)
	tick(e, ladder...)

	scenario(4, "L1 empty, L2 and L3 rostered, nobody answers", first+", the handoff message, the card at once with \"Nobody rostered before L2\", Nimal Fernando (L2) and the time, then L2 and L3 replies and the unanswered closing message.")
	e = engine(full[1:])
	p = alert(4, "Apollo", "asgardeo", testClock)
	open(e, p)
	escalate(e, p)
	tick(e, ladder...)

	scenario(5, "L1 empty, assigned before L2", first+", the handoff message, the card naming L2, then a stopped reply with no SME rung replies.")
	e = engine(full[1:])
	p = alert(5, "Apollo", "asgardeo", testClock)
	open(e, p)
	escalate(e, p)
	tick(e, 0)
	assign(e, p.IncidentID, "Nimal Fernando", testClock.Add(2*time.Minute))
	tick(e, ladder[1:]...)

	scenario(6, "only L3 rostered", first+", the handoff message, the card with \"Nobody rostered before L3\" and Dilini Wickramasinghe (L3), then the L3 reply after ten minutes and the unanswered closing message.")
	e = engine(full[2:])
	p = alert(6, "Apollo", "asgardeo", testClock)
	open(e, p)
	escalate(e, p)
	tick(e, ladder...)

	scenario(7, "the same SME team escalated twice", first+", one handoff message, one card and one L1 reply. The second escalation is ignored.")
	e = engine(full)
	p = alert(7, "Apollo", "asgardeo", testClock)
	open(e, p)
	escalate(e, p)
	tick(e, 0)
	escalate(e, alert(7, "Apollo", "asgardeo", testClock.Add(time.Minute)))
	tick(e, time.Minute)

	scenario(8, "escalated from an IaaS SRE team", "the SRE \"Incident Reported\" card with its L1 reply (Ishara Bandara), then nothing more. Only SaaS SRE incidents page the SMEs.")
	e = engine(full)
	p = alert(8, "SRE IaaS", "asgardeo", testClock)
	open(e, p)
	escalate(e, p)
	tick(e, 0)

	scenario(9, "no SME team mapped", first+", then nothing more. SRE paging stops and only a NO_SME_TEAM work note is written.")
	e = engine(full)
	p = alert(9, "Apollo", "", testClock)
	p.TeamKey, p.TeamLabel = "devant", "Devant"
	open(e, p)
	escalate(e, p)
	tick(e, 0)

	scenario(10, "assignment handled before the escalation", first+", then \"Incident Acknowledged.\" from the assignment, then nothing more. The escalation was already answered.")
	e = engine(full)
	p = alert(10, "Apollo", "asgardeo", testClock)
	open(e, p)
	assign(e, p.IncidentID, "Kasun Perera", testClock.Add(time.Minute))
	escalate(e, p)
	tick(e, ladder...)

	say("*SME escalation testing finished.*")
	t.Logf("webhook requests: %d, statuses: %v", len(th.statuses), th.statuses)
}
