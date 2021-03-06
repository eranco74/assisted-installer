package inventory_client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/eranco74/assisted-installer/src/utils"
	"github.com/go-openapi/strfmt"

	"github.com/openshift/assisted-service/client"
	"github.com/openshift/assisted-service/client/installer"
	"github.com/openshift/assisted-service/models"
	"github.com/openshift/assisted-service/pkg/auth"
	"github.com/openshift/assisted-service/pkg/requestid"
	"github.com/sirupsen/logrus"
)

//go:generate mockgen -source=inventory_client.go -package=inventory_client -destination=mock_inventory_client.go
type InventoryClient interface {
	DownloadFile(filename string, dest string) error
	UpdateHostInstallProgress(hostId string, newStage models.HostStage, info string) error
	GetEnabledHostsNamesHosts() (map[string]EnabledHostData, error)
	UploadIngressCa(ingressCA string, clusterId string) error
	GetCluster() (*models.Cluster, error)
	CompleteInstallation(clusterId string, isSuccess bool, errorInfo string) error
}

type inventoryClient struct {
	log       *logrus.Logger
	ai        *client.AssistedInstall
	clusterId strfmt.UUID
}

type EnabledHostData struct {
	IPs       []string
	Inventory *models.Inventory
	Host      *models.Host
}

func CreateInventoryClient(clusterId string, inventoryURL string, pullSecret string, logger *logrus.Logger) (*inventoryClient, error) {
	clientConfig := client.Config{}
	var err error
	clientConfig.URL, err = url.ParseRequestURI(createUrl(inventoryURL))
	if err != nil {
		return nil, err
	}

	clientConfig.Transport = requestid.Transport(http.DefaultTransport)
	clientConfig.AuthInfo = auth.AgentAuthHeaderWriter(pullSecret)
	assistedInstallClient := client.New(clientConfig)
	return &inventoryClient{logger, assistedInstallClient, strfmt.UUID(clusterId)}, nil
}

func (c *inventoryClient) DownloadFile(filename string, dest string) error {
	// open output file
	fo, err := os.Create(dest)
	if err != nil {
		return err
	}
	// close fo on exit and check for its returned error
	defer func() {
		fo.Close()
	}()
	_, err = c.ai.Installer.DownloadClusterFiles(context.Background(), c.createDownloadParams(filename), fo)
	return err
}

func (c *inventoryClient) UpdateHostInstallProgress(hostId string, newStage models.HostStage, info string) error {
	_, err := c.ai.Installer.UpdateHostInstallProgress(context.Background(), c.createUpdateHostInstallProgressParams(hostId, newStage, info))
	return err
}

func (c *inventoryClient) UploadIngressCa(ingressCA string, clusterId string) error {
	_, err := c.ai.Installer.UploadClusterIngressCert(context.Background(),
		&installer.UploadClusterIngressCertParams{ClusterID: strfmt.UUID(clusterId), IngressCertParams: models.IngressCertParams(ingressCA)})
	return err
}

func (c *inventoryClient) GetCluster() (*models.Cluster, error) {
	cluster, err := c.ai.Installer.GetCluster(context.Background(), &installer.GetClusterParams{ClusterID: c.clusterId})
	if err != nil {
		return nil, err
	}

	return cluster.Payload, nil
}

func (c *inventoryClient) GetEnabledHostsNamesHosts() (map[string]EnabledHostData, error) {
	namesIdsMap := make(map[string]EnabledHostData)
	hosts, err := c.getEnabledHostsWithInventoryInfo()
	if err != nil {
		return nil, err
	}
	for _, hostData := range hosts {
		hostname := hostData.Host.RequestedHostname
		ips, err := utils.GetHostIpsFromInventory(hostData.Inventory)
		if err != nil {
			c.log.WithError(err).Errorf("failed to get ips of node %s", hostname)
		}
		hostData.IPs = ips
		namesIdsMap[hostname] = hostData
	}
	return namesIdsMap, nil
}

func createUrl(baseURL string) string {
	return fmt.Sprintf("%s/%s",
		baseURL,
		client.DefaultBasePath,
	)
}

func (c *inventoryClient) createDownloadParams(filename string) *installer.DownloadClusterFilesParams {
	return &installer.DownloadClusterFilesParams{
		ClusterID: c.clusterId,
		FileName:  filename,
	}
}

func (c *inventoryClient) createUpdateHostInstallProgressParams(hostId string, newStage models.HostStage, info string) *installer.UpdateHostInstallProgressParams {
	return &installer.UpdateHostInstallProgressParams{
		ClusterID: c.clusterId,
		HostID:    strfmt.UUID(hostId),
		HostProgress: &models.HostProgress{
			CurrentStage: newStage,
			ProgressInfo: info,
		},
	}
}

func (c *inventoryClient) getEnabledHostsWithInventoryInfo() (map[string]EnabledHostData, error) {
	hostsWithHwInfo := make(map[string]EnabledHostData)
	hosts, err := c.ai.Installer.ListHosts(context.Background(), &installer.ListHostsParams{ClusterID: c.clusterId})
	if err != nil {
		return nil, err
	}
	for _, host := range hosts.Payload {
		if *host.Status == models.HostStatusDisabled {
			continue
		}
		hwInfo := models.Inventory{}
		err = json.Unmarshal([]byte(host.Inventory), &hwInfo)
		if err != nil {
			c.log.Warnf("Failed to parse host %s inventory %s", host.ID.String(), host.Inventory)
			return nil, err
		}
		hostsWithHwInfo[host.ID.String()] = EnabledHostData{Inventory: &hwInfo, Host: host}
	}
	return hostsWithHwInfo, nil
}

func (c *inventoryClient) CompleteInstallation(clusterId string, isSuccess bool, errorInfo string) error {
	_, err := c.ai.Installer.CompleteInstallation(context.Background(),
		&installer.CompleteInstallationParams{ClusterID: strfmt.UUID(clusterId),
			CompletionParams: &models.CompletionParams{IsSuccess: &isSuccess, ErrorInfo: errorInfo}})
	return err
}
