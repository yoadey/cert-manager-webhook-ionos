package ionos

import (
	"context"
	"fmt"
)

func (e *ionosSolver) GetZoneIdByName(ctx context.Context, name string) (string, error) {

	zones, _, err := e.ionosClient.ZonesApi.ZonesGet(e.context).FilterZoneName(name).Execute()
	if err != nil {
		return "", fmt.Errorf("unable to get zone info %v", err)
	}
	for _, zone := range *zones.Items {
		if *zone.Properties.ZoneName == name {
			return *zone.Id, nil
		}
	}

	return "", fmt.Errorf("unable to find zone %v", name)
}

func (e *ionosSolver) GetRecordIdByName(ctx context.Context, zoneId string, name string) (string, error) {

	records, _, err := e.ionosClient.RecordsApi.RecordsGet(e.context).FilterName(name).Execute()
	if err != nil {
		return "", fmt.Errorf("unable to get zone info %v", err)
	}
	for _, record := range *records.Items {
		if *record.Properties.Name == name {
			return *record.Id, nil
		}
	}

	return "", fmt.Errorf("unable to find zone %v", name)
}
