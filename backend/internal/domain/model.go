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
	DisplayName   string  `json:"displayName"`
	RoleID        string  `json:"roleId"`
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

// RoleProfile is the user-facing intent for a port. Device-specific commands
// are derived from this profile so operators never need to handle VLAN tagging,
// QoS or EEE during normal operation.
type RoleProfile struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Color          string   `json:"color"`
	Icon           string   `json:"icon"`
	VLANID         int      `json:"vlanId"`
	PortMode       string   `json:"portMode"`
	Multicast      bool     `json:"multicast"`
	DanteQoS       bool     `json:"danteQos"`
	DisableEEE     bool     `json:"disableEee"`
	PoEMode        string   `json:"poeMode"`
	AllowedRoleIDs []string `json:"allowedRoleIds"`
}

type PortSetting struct {
	SwitchID    string `json:"switchId"`
	PortIndex   int    `json:"portIndex"`
	DisplayName string `json:"displayName"`
	RoleID      string `json:"roleId"`
}

type RolePortRequest struct {
	PortIndex   int    `json:"portIndex"`
	DisplayName string `json:"displayName"`
	RoleID      string `json:"roleId"`
}

type RolePlanRequest struct {
	SwitchID string            `json:"switchId"`
	Ports    []RolePortRequest `json:"ports"`
}

type RoleApplyRequest struct {
	Plan     ConfigPlan        `json:"plan"`
	Settings []RolePortRequest `json:"settings"`
}

func DefaultRoleProfiles() []RoleProfile {
	return []RoleProfile{
		{ID: "dante", Name: "Dante/Audio", Description: "Für digitale Audionetzwerke: priorisiert Audio und schützt Multicast-Streams.", Color: "#9168ff", Icon: "♪", VLANID: 1, PortMode: "access", Multicast: true, DanteQoS: true, DisableEEE: true, PoEMode: "auto"},
		{ID: "control", Name: "Control", Description: "Für Yamaha-Steuerung, Editoren, Controller und Geräteverwaltung – getrennt vom Audionetz.", Color: "#23a7ff", Icon: "⌁", VLANID: 2, PortMode: "access", PoEMode: "auto"},
		{ID: "lighting", Name: "Lighting", Description: "Für MA-Net2/3, Art-Net und sACN mit stabiler Multicast-Verteilung.", Color: "#f0aa34", Icon: "✦", VLANID: 3, PortMode: "access", Multicast: true, DisableEEE: true, PoEMode: "auto"},
		{ID: "internet", Name: "Internet", Description: "Für Internetzugang und allgemeine Datengeräte.", Color: "#2ed47a", Icon: "◎", VLANID: 4, PortMode: "access", PoEMode: "auto"},
		{ID: "video", Name: "Video", Description: "Für Video-over-IP und andere bandbreitenintensive Multicast-Signale.", Color: "#ed5d86", Icon: "▶", VLANID: 5, PortMode: "access", Multicast: true, DisableEEE: true, PoEMode: "auto"},
		{ID: "trunk", Name: "Trunk/Management", Description: "Für Switch-Uplinks: transportiert alle Rollen und das Management-Netz gemeinsam.", Color: "#5ea8ff", Icon: "⇄", VLANID: 4000, PortMode: "trunk", DisableEEE: true, PoEMode: "off", AllowedRoleIDs: []string{"dante", "control", "lighting", "internet", "video"}},
	}
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
	TargetDeviceID string `json:"targetDeviceId,omitempty"`
	Protocol       string `json:"protocol"`
}

type ConnectedDevice struct {
	ID            string `json:"id"`
	SwitchID      string `json:"switchId"`
	PortIndex     int    `json:"portIndex"`
	Name          string `json:"name"`
	IPAddress     string `json:"ipAddress,omitempty"`
	MACAddress    string `json:"macAddress,omitempty"`
	Model         string `json:"model,omitempty"`
	SuggestedRole string `json:"suggestedRole,omitempty"`
	Protocol      string `json:"protocol"`
}

type Topology struct {
	Switches  []Switch          `json:"switches"`
	Links     []Link            `json:"links"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Source    string            `json:"source"`
	VLANs     []VLAN            `json:"vlans"`
	Devices   []ConnectedDevice `json:"devices"`
}

type SwitchNameRequest struct {
	SwitchID string `json:"switchId"`
	Name     string `json:"name"`
}

type SwitchConfigState struct {
	SwitchID     string        `json:"switchId"`
	Name         string        `json:"name"`
	PortSettings []PortSetting `json:"portSettings"`
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

type EventBaselineCheck struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	OK          bool   `json:"ok"`
}

type EventBaselineStatus struct {
	SwitchID  string               `json:"switchId"`
	CheckedAt time.Time            `json:"checkedAt"`
	Healthy   bool                 `json:"healthy"`
	Checks    []EventBaselineCheck `json:"checks"`
}

type Snapshot struct {
	ID            string    `json:"id"`
	SwitchID      string    `json:"switchId"`
	CreatedAt     time.Time `json:"createdAt"`
	Configuration string    `json:"-"`
	SizeBytes     int       `json:"sizeBytes"`
}

type Alarm struct {
	ID             string    `json:"id"`
	Severity       string    `json:"severity"`
	Category       string    `json:"category"`
	SwitchID       string    `json:"switchId"`
	SwitchName     string    `json:"switchName"`
	PortIndex      int       `json:"portIndex,omitempty"`
	PortName       string    `json:"portName,omitempty"`
	Title          string    `json:"title"`
	Message        string    `json:"message"`
	Recommendation string    `json:"recommendation"`
	CurrentValue   float64   `json:"currentValue,omitempty"`
	Threshold      float64   `json:"threshold,omitempty"`
	DetectedAt     time.Time `json:"detectedAt"`
}

type AlarmReport struct {
	GeneratedAt   time.Time `json:"generatedAt"`
	HealthPercent int       `json:"healthPercent"`
	CriticalCount int       `json:"criticalCount"`
	WarningCount  int       `json:"warningCount"`
	InfoCount     int       `json:"infoCount"`
	ChecksOK      []string  `json:"checksOk"`
	Alarms        []Alarm   `json:"alarms"`
}
