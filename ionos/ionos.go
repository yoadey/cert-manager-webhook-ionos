package ionos

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/issuer/acme/dns/util"
	ionos "github.com/ionos-cloud/sdk-go-dns"
)

// ionosDNSProviderSolver implements the provider-specific logic needed to
// 'present' an ACME challenge TXT record for your own DNS provider.
// To do so, it must implement the `github.com/cert-manager/cert-manager/pkg/acme/webhook.Solver`
// interface.
type ionosDNSProviderSolver struct {
	client *kubernetes.Clientset
}

func NewIonosDNSProviderSeolver() *ionosDNSProviderSolver {
	return &ionosDNSProviderSolver{}
}

// ionosDNSProviderConfig is a structure that is used to decode into when
// solving a DNS01 challenge.
// This information is provided by cert-manager, and may be a reference to
// additional configuration that's needed to solve the challenge for this
// particular certificate or issuer.
// This typically includes references to Secret resources containing DNS
// provider credentials, in cases where a 'multi-tenant' DNS solver is being
// created.
// If you do *not* require per-issuer or per-certificate configuration to be
// provided to your webhook, you can skip decoding altogether in favour of
// using CLI flags or similar to provide configuration.
// You should not include sensitive information here. If credentials need to
// be used by your provider here, you should reference a Kubernetes Secret
// resource and fetch these credentials using a Kubernetes clientset.
type ionosDNSProviderConfig struct {
	Endpoint          string                   `json:"endpoint"`
	ApiTokenSecretRef corev1.SecretKeySelector `json:"apiTokenSecretRef"`
}

type ionosZoneStatus struct {
	IsDeployed bool `json:"isDeployed"`
}

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource.
// This should be unique **within the group name**, i.e. you can have two
// solvers configured with the same Name() **so long as they do not co-exist
// within a single webhook deployment**.
// For example, `cloudflare` may be used as the name of a solver.
func (s *ionosDNSProviderSolver) Name() string {
	return "ionos"
}

func (s *ionosDNSProviderSolver) validate(cfg *ionosDNSProviderConfig, allowAmbientCredentials bool) error {
	if allowAmbientCredentials {
		// When allowAmbientCredentials is true, ionos client can load missing config
		// values from the environment variables and the ionos.conf files.
		return nil
	}
	if cfg.Endpoint == "" {
		return errors.New("no endpoint provided in ionos config")
	}
	if cfg.ApiTokenSecretRef.Name == "" {
		return errors.New("no api token secret provided in ionos config")
	}
	return nil
}

func (s *ionosDNSProviderSolver) ionosClient(ch *v1alpha1.ChallengeRequest) (*ionos.APIClient, error) {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return nil, err
	}

	err = s.validate(&cfg, ch.AllowAmbientCredentials)
	if err != nil {
		return nil, err
	}

	apiToken, err := s.secret(cfg.ApiTokenSecretRef, ch.ResourceNamespace)
	if err != nil {
		return nil, err
	}

	client := ionos.NewAPIClient(ionos.NewConfiguration("", "", apiToken, cfg.Endpoint))

	return client, nil
}

func (s *ionosDNSProviderSolver) secret(ref corev1.SecretKeySelector, namespace string) (string, error) {
	if ref.Name == "" {
		return "", nil
	}

	secret, err := s.client.CoreV1().Secrets(namespace).Get(context.Background(), ref.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	bytes, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key not found %q in secret '%s/%s'", ref.Key, namespace, ref.Name)
	}
	return string(bytes), nil
}

// Present is responsible for actually presenting the DNS record with the
// DNS provider.
// This method should tolerate being called multiple times with the same value.
// cert-manager itself will later perform a self check to ensure that the
// solver has correctly configured the DNS provider.
func (s *ionosDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	ionosClient, err := s.ionosClient(ch)
	if err != nil {
		return err
	}
	domain := util.UnFqdn(ch.ResolvedZone)
	subDomain := getSubDomain(domain, ch.ResolvedFQDN)
	target := ch.Key
	return addTXTRecord(ionosClient, domain, subDomain, target)
}

// CleanUp should delete the relevant TXT record from the DNS provider console.
// If multiple TXT records exist with the same record name (e.g.
// _acme-challenge.example.com) then **only** the record with the same `key`
// value provided on the ChallengeRequest should be cleaned up.
// This is in order to facilitate multiple DNS validations for the same domain
// concurrently.
func (s *ionosDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	ionosClient, err := s.ionosClient(ch)
	if err != nil {
		return err
	}
	domain := util.UnFqdn(ch.ResolvedZone)
	subDomain := getSubDomain(domain, ch.ResolvedFQDN)
	target := ch.Key
	return removeTXTRecord(ionosClient, domain, subDomain, target)
}

// Initialize will be called when the webhook first starts.
// This method can be used to instantiate the webhook, i.e. initialising
// connections or warming up caches.
// Typically, the kubeClientConfig parameter is used to build a Kubernetes
// client that can be used to fetch resources from the Kubernetes API, e.g.
// Secret resources containing credentials used to authenticate with DNS
// provider accounts.
// The stopCh can be used to handle early termination of the webhook, in cases
// where a SIGTERM or similar signal is sent to the webhook process.
func (s *ionosDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	client, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}

	s.client = client
	return nil
}

func addTXTRecord(ionosClient *ionos.APIClient, domain, subDomain, target string) error {
	klog.Warningf("(Warntest) Ensuring TXT record for domain %s.%s with value %s", subDomain, domain, target)
	klog.Infof("Ensuring TXT record for domain %s.%s with value %s", subDomain, domain, target)
	err := validateZone(ionosClient, domain)
	if err != nil {
		return err
	}
	zoneId, err := findZoneId(ionosClient, domain)
	if err != nil {
		return err
	}

	// We don't need to create the record, if it already exists
	record, _ := findRecord(ionosClient, *zoneId, subDomain, target)
	if record == nil {
		_, err = createRecord(ionosClient, *zoneId, "TXT", subDomain, target)
		klog.Infof("TXT record created for domain %s.%s with value %s", subDomain, domain, target)
		if err != nil {
			return err
		}
	} else {
		klog.Infof("TXT record already present for domain %s.%s with value %s", subDomain, domain, target)
	}
	return nil
}

func removeTXTRecord(ionosClient *ionos.APIClient, domain, subDomain, target string) error {
	zoneId, err := findZoneId(ionosClient, domain)
	if err != nil {
		return err
	}

	record, _ := findRecord(ionosClient, *zoneId, subDomain, target)
	if record != nil {
		_, err := ionosClient.RecordsApi.ZonesRecordsDelete(context.Background(), *zoneId, *record.Id).Execute()
		if err != nil {
			return err
		}
	}

	return nil
}

func findRecord(ionosClient *ionos.APIClient, zoneId, subDomain, target string) (*ionos.RecordRead, error) {
	records, err := listRecords(ionosClient, zoneId, "TXT", subDomain)
	if err != nil {
		return nil, err
	}

	for _, record := range *records.Items {
		if err != nil {
			return nil, err
		}
		if *record.Properties.Name == subDomain && *record.Properties.Content == target {
			return &record, nil
		}
	}

	return nil, fmt.Errorf("No record found for zoneId '%s', subdomain '%s' and target '%s'", zoneId, subDomain, target)
}

func validateZone(ionosClient *ionos.APIClient, domain string) error {
	klog.Warningf("Validating Zone for domain %s", domain)

	zones, _, err := ionosClient.ZonesApi.ZonesGet(context.Background()).FilterZoneName(domain).Execute()
	if err != nil {
		return fmt.Errorf("ionos API call failed: %v", err)
	}
	if len(*zones.Items) != 1 {
		return fmt.Errorf("No ionos zone found for domain %s", domain)
	}
	if *(*zones.Items)[0].Metadata.State != ionos.AVAILABLE {
		return fmt.Errorf("ionos zone not deployed for domain %s", domain)
	}

	return nil
}

func findZoneId(ionosClient *ionos.APIClient, domain string) (*string, error) {
	zones, _, err := ionosClient.ZonesApi.ZonesGet(context.Background()).FilterZoneName(domain).Execute()
	if err != nil {
		return nil, fmt.Errorf("Unable to find zone id for domain '%s', ionos API call failed: %v", domain, err)
	}
	if len(*zones.Items) != 1 {
		return nil, fmt.Errorf("No ionos zone found for domain %s", domain)
	}
	return (*zones.Items)[0].Id, nil
}

func listRecords(ionosClient *ionos.APIClient, zoneId, fieldType, subDomain string) (*ionos.RecordReadList, error) {
	records, _, err := ionosClient.RecordsApi.RecordsGet(context.Background()).FilterZoneId(zoneId).Execute()
	if err != nil {
		return nil, fmt.Errorf("ionos API call failed: %v", err)
	}
	return &records, nil
}

// Ionos API doesn't allow to create DNS records whichs name contain a "_", but it's possible to do via a zonefile
//func createRecord(ionosClient *ionos.APIClient, zoneId, fieldType, subDomain, target string) (*ionos.RecordRead, error) {
//
//	record := ionos.NewRecordCreate(*ionos.NewRecord(subDomain, fieldType, target))
//
//	recordResult, _, err := ionosClient.RecordsApi.ZonesRecordsPost(context.Background(), zoneId).RecordCreate(*record).Execute()
//
//	if err != nil {
//		return nil, fmt.Errorf("ionos API call failed: create record %s/%s %v", zoneId, subDomain, err)
//	}
//
//	return &recordResult, nil
//}
//

func createRecord(ionosClient *ionos.APIClient, zoneId, fieldType, subDomain, target string) (*ionos.RecordReadList, error) {
	zoneFile, err := retrieveZoneFile(ionosClient, zoneId)
	if err != nil {
		return nil, fmt.Errorf("Unable to create record '%s/%s': ionos API call to retrieve zonefile failed: %v", zoneId, subDomain, err)
	}
	if !strings.HasPrefix(*zoneFile, ";Zone: ") {
		return nil, fmt.Errorf("Retrieved zone file has not expected Format: %s", *zoneFile)
	}

	newLine := fmt.Sprintf("%s	%d	IN	TXT	\"%s\"", subDomain, 60, target)
	if !strings.Contains(*zoneFile, newLine) {
		// We don't need to create the record, if it already exists
		newZoneFile := *zoneFile + "\n" + newLine
		uploadZoneFile(ionosClient, zoneId, newZoneFile)
		if err != nil {
			return nil, fmt.Errorf("Unable to create record '%s/%s': ionos API call to update zonefile failed: %v", zoneId, subDomain, err)
		}
	}

	records, err := listRecords(ionosClient, zoneId, "TXT", subDomain)

	if err != nil {
		return nil, fmt.Errorf("Unable to create record '%s/%s': ionos API call failed: %v", zoneId, subDomain, err)
	}

	return records, nil
}
