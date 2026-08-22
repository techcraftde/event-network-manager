package domain

import "time"

type Switch struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Model           string  `json:"model"`
	Address         string  `json:"address"`
	Status          string  `json:"status"`
	FirmwareVersion string  `json:"firmwareVersion"`
	HardwareVersion string  `json:"hardwareVersion"`
	SerialNumber    string  `json:"serialNumber"`
	MACAddress      string  `json:"macAddress"`
	UptimeSeconds   int64   `json:"uptimeSeconds"`
	CPUPercent      float64 `json:"cpuPercent"`
	TemperatureC    float64 `json:"temperatureC"`
	PoEBudgetWatts  float64 `json:"poeBudgetWatts"`
	PoEUsageWatts   float64 `json:"poeUsageWatts"`
	Ports           []Port  `json:"ports"`
}

type Port struct {
	Index         int     `json:"index"`
	Name          string  `json:"name"`
	Link          bool    `json:"link"`
	SpeedMbps     int     `json:"speedMbps"`
	Role          string  `json:"role"`
	VLANs         []int   `json:"vlans"`
	PVID          int     `json:"pvid"`
	TaggedVLANs   []int   `json:"taggedVlans"`
	UntaggedVLANs []int   `json:"untaggedVlans"`
	VLANMode      string  `json:"vlanMode"`
	RxMbps        float64 `json:"rxMbps"`
	TxMbps        float64 `json:"txMbps"`
	Errors        int64   `json:"errors"`
	PoEWatts      float64 `json:"poeWatts"`
	PoEEnabled    bool    `json:"poeEnabled"`
}

type VLAN struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Link struct {
	ID             string `json:"id"`
	SourceSwitchID string `json:"sourceSwitchId"`
	SourcePort     int    `json:"sourcePort"`
	TargetSwitchID string `json:"targetSwitchId"`
	TargetPort     int    `json:"targetPort"`
	Protocol       string `json:"protocol"`
}

type Topology struct {
	Switches  []Switch  `json:"switches"`
	Links     []Link    `json:"links"`
	UpdatedAt time.Time `json:"updatedAt"`
	Source    string    `json:"source"`
	VLANs     []VLAN    `json:"vlans"`
}

type ConfigChange struct {
	SwitchID    string   `json:"switchId"`
	Description string   `json:"description"`
	Commands    []string `json:"commands"`
}

type ConfigStatus struct {
	SwitchID           string `json:"switchId"`
	Available          bool   `json:"available"`
	HostKeyTrusted     bool   `json:"hostKeyTrusted"`
	HostKeyAlgorithm   string `json:"hostKeyAlgorithm,omitempty"`
	HostKeyFingerprint string `json:"hostKeyFingerprint,omitempty"`
	Message            string `json:"message,omitempty"`
}

type HostKeyTrust struct {
	SwitchID    string `json:"switchId"`
	Fingerprint string `json:"fingerprint"`
}

type DantePlanRequest struct {
	SwitchID string `json:"switchId"`
	VLANID   int    `json:"vlanId"`
	Ports    []int  `json:"ports"`
	Apply    bool   `json:"apply"`
}

type VLANPlanRequest struct {
	SwitchID string `json:"switchId"`
	VLANID   int    `json:"vlanId"`
	Ports    []int  `json:"ports"`
	Mode     string `json:"mode"`
}

type DanteHealthRequest struct {
	SwitchID string `json:"switchId"`
	VLANID   int    `json:"vlanId"`
	Ports    []int  `json:"ports"`
}

type DanteHealth struct {
	SwitchID         string    `json:"switchId"`
	VLANID           int       `json:"vlanId"`
	CheckedAt        time.Time `json:"checkedAt"`
	IGMPGlobal       bool      `json:"igmpGlobal"`
	IGMPVLAN         bool      `json:"igmpVlan"`
	QoSDSCP          bool      `json:"qosDscp"`
	SelectedPorts    int       `json:"selectedPorts"`
	QoSTrustedPorts  int       `json:"qosTrustedPorts"`
	EEEDisabledPorts int       `json:"eeeDisabledPorts"`
	Healthy          bool      `json:"healthy"`
	Messages         []string  `json:"messages"`
}

type ConfigPlan struct {
	ID          string   `json:"id"`
	SwitchID    string   `json:"switchId"`
	Description string   `json:"description"`
	Commands    []string `json:"commands"`
	Warnings    []string `json:"warnings"`
}

type Snapshot struct {
	ID            string    `json:"id"`
	SwitchID      string    `json:"switchId"`
	CreatedAt     time.Time `json:"createdAt"`
	Configuration string    `json:"-"`
	SizeBytes     int       `json:"sizeBytes"`
}
