// Package protocol defines the public, path-free companion contract.
package protocol

import "encoding/json"

const MaxBytes = 2 * 1024 * 1024
const MaxGameIDs = 10000
const MaxCollections = 512
const Version = 1

type Collection struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Status string   `json:"status"`
	AppIDs []uint32 `json:"-"`
}

func (c Collection) MarshalJSON() ([]byte, error) {
	if c.Status == "supported" {
		ids := c.AppIDs
		if ids == nil {
			ids = []uint32{}
		}
		return json.Marshal(struct {
			ID     string   `json:"id"`
			Name   string   `json:"name"`
			Status string   `json:"status"`
			AppIDs []uint32 `json:"appids"`
		}{c.ID, c.Name, c.Status, ids})
	}
	return json.Marshal(struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}{c.ID, c.Name, c.Status})
}

type Snapshot struct {
	Protocol    int          `json:"protocol"`
	InstanceID  string       `json:"instance_id"`
	SourceID    string       `json:"source_id"`
	SteamID     string       `json:"steam_id"`
	Revision    string       `json:"revision"`
	ReadAt      string       `json:"read_at"`
	Collections []Collection `json:"collections"`
}

type SnapshotResponse struct {
	Protocol    int       `json:"protocol"`
	State       string    `json:"state"`
	AttemptedAt string    `json:"attempted_at"`
	Error       *string   `json:"error"`
	Snapshot    *Snapshot `json:"snapshot"`
}
