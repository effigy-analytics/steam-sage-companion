package steam

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/effigy-analytics/steam-sage-companion/internal/protocol"
)

func Read(account Account) (protocol.Snapshot, error) {
	file, err := account.collectionPath()
	if err != nil {
		return protocol.Snapshot{}, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		f, err := os.Open(file)
		if os.IsNotExist(err) {
			return protocol.Snapshot{}, ErrMissing
		}
		if err != nil {
			return protocol.Snapshot{}, ErrUnreadable
		}
		before, err := f.Stat()
		if err != nil || !before.Mode().IsRegular() {
			f.Close()
			return protocol.Snapshot{}, ErrUnreadable
		}
		data, err := io.ReadAll(io.LimitReader(f, protocol.MaxBytes+1))
		after, statErr := f.Stat()
		f.Close()
		if err != nil || statErr != nil {
			return protocol.Snapshot{}, ErrUnreadable
		}
		if len(data) > protocol.MaxBytes {
			return protocol.Snapshot{}, ErrLimit
		}
		current, err := os.Stat(file)
		if err != nil || !os.SameFile(before, current) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || !after.ModTime().Equal(current.ModTime()) || current.Size() != int64(len(data)) {
			continue
		}
		collections, err := ParseCollections(data)
		if err != nil {
			return protocol.Snapshot{}, err
		}
		encoded, _ := json.Marshal(collections)
		sum := sha256.Sum256(encoded)
		result := protocol.Snapshot{Protocol: 1, SteamID: account.SteamID, Revision: hex.EncodeToString(sum[:]), ReadAt: time.Now().UTC().Format(time.RFC3339Nano), Collections: collections}
		encoded, _ = json.Marshal(result)
		if len(encoded) > protocol.MaxBytes {
			return protocol.Snapshot{}, ErrLimit
		}
		return result, nil
	}
	return protocol.Snapshot{}, ErrChanging
}
