package core

import "github.com/LumeWeb/portal/db/models"

type DNSLinkService interface {
	// DNSLinkExists checks if a DNS link exists for the given hash.
	// It returns a boolean indicating if the link exists, the DNSLink model,
	// and an error if any.
	DNSLinkExists(hash []byte) (bool, *models.DNSLink, error)
}
