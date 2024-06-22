package event

import "go.lumeweb.com/portal/core"

const (
	EVENT_USER_SUBDOMAIN_SET = "user.subdomain.set"
)

func init() {
	core.RegisterEvent(EVENT_USER_SUBDOMAIN_SET, &UserSubdomainSetEvent{})
}

type UserSubdomainSetEvent struct {
	core.Event
}

func (e *UserSubdomainSetEvent) SetSubdomain(subdomain string) {
	e.Set("subdomain", subdomain)
}

func (e UserSubdomainSetEvent) Subdomain() string {
	return e.Get("subdomain").(string)
}
