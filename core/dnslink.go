package core

import "go.lumeweb.com/portal/db/models"

const DNSLINK_SERVICE = "dnslink"

type DNSLinkService interface {
	// DNSLinkExists checks if a DNS link exists for the given hash.
	// It returns a boolean indicating if the link exists, the DNSLink model,
	// and an error if any.
	DNSLinkExists(hash StorageHash) (bool, *models.DNSLink, error)

	Service
}
