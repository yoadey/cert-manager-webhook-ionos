package ionos

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"

	"k8s.io/klog/v2"

	ionos "github.com/ionos-cloud/sdk-go-dns"
)

// This file contains functions which we would have assumed to be in the
// ionos sdk, but are missing there.
// Also, they are only required because the records API does not accept
// names with underscore '_' like '_acme-challenge.example.com'

func retrieveZoneFile(ionosClient *ionos.APIClient, zoneId string) (*string, error) {
	config := ionosClient.GetConfig()
	if config == nil {
		return nil, fmt.Errorf("config missing")
	}
	url := config.Servers[0].URL + "/zones/" + zoneId + "/zonefile"
	return executeApiRequest(ionosClient, "GET", url, "")
}

func uploadZoneFile(ionosClient *ionos.APIClient, zoneId, zoneFile string) (*string, error) {
	config := ionosClient.GetConfig()
	if config == nil {
		return nil, fmt.Errorf("config missing")
	}
	url := config.Servers[0].URL + "/zones/" + zoneId + "/zonefile"
	return executeApiRequest(ionosClient, "PUT", url, zoneFile)
}

func executeApiRequest(ionosClient *ionos.APIClient, method, url, body string) (*string, error) {
	config := ionosClient.GetConfig()
	if config == nil {
		return nil, fmt.Errorf("config missing")
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer([]byte(body)))
	if err != nil {
		return nil, fmt.Errorf("unable to execute request %v", err)
	}

	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.Token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			klog.Fatal(err)
		}
	}()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		body := string(respBody)
		return &body, nil
	}

	text := "Error calling API. Status:" + resp.Status + " url: " + url + " method: " + method
	klog.Error(text)
	return nil, errors.New(text)
}
