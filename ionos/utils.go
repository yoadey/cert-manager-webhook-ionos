package ionos

import (
	"encoding/json"
	"fmt"
	"strings"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/cert-manager/cert-manager/pkg/issuer/acme/dns/util"
)

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (ionosDNSProviderConfig, error) {
	cfg := ionosDNSProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding ionos config: %v", err)
	}

	return cfg, nil
}

func getSubDomain(domain, fqdn string) string {
	if idx := strings.Index(fqdn, "."+domain); idx != -1 {
		return fqdn[:idx]
	}

	return util.UnFqdn(fqdn)
}
