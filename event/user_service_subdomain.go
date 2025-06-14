package event

const EVENT_USER_SERVICE_SUBDOMAIN_SET = "user.subdomain.set"

type UserServiceSubdomainSetEvent struct {
	Subdomain string
}

func NewUserServiceSubdomainSetEvent(subdomain string) *UserServiceSubdomainSetEvent {
	return &UserServiceSubdomainSetEvent{
		Subdomain: subdomain,
	}
}
