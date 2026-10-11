package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		for _, name := range []string{"agent_machine_fingerprint", "agent_remote_uid", "agent_install_dir", "agent_instance_id"} {
			if collection.Fields.GetByName(name) == nil {
				collection.Fields.Add(&core.TextField{Name: name})
			}
		}
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("systems")
		if err != nil {
			return err
		}
		for _, name := range []string{"agent_machine_fingerprint", "agent_remote_uid", "agent_install_dir", "agent_instance_id"} {
			collection.Fields.RemoveByName(name)
		}
		return app.Save(collection)
	})
}
