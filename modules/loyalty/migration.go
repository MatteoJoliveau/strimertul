package loyalty

import (
	"github.com/sirupsen/logrus"
	"github.com/strimertul/strimertul/database"
)

const OldPointsKey = "loyalty/users"

type OldPointStorage map[string]int64

func migratePoints(db *database.DB, log logrus.FieldLogger) error {
	// Retrieve old storage
	var oldStorage OldPointStorage
	err := db.GetJSON(OldPointsKey, &oldStorage)
	if err == database.ErrKeyNotFound {
		// No migration needed, points are already kaput
		return nil
	}
	if err != nil {
		return err
	}

	// Move to new format
	newStorage := make(map[string]interface{})
	for user, points := range oldStorage {
		userkey := PointsPrefix + user
		newStorage[userkey] = PointsEntry{
			Points: points,
		}
	}

	// Bulk add to database
	if err := db.PutJSONBulk(newStorage); err != nil {
		return err
	}

	log.Info("Migrated to new loyalty point format")

	// Remove old key
	return db.RemoveKey(OldPointsKey)
}