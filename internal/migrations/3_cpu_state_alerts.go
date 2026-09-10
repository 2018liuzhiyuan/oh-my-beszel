package migrations

import (
	"fmt"
	"slices"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		field, ok := collection.Fields.GetByName("name").(*core.SelectField)
		if !ok {
			return fmt.Errorf("alerts.name must be a select field")
		}
		for _, name := range []string{"CPUIOWait", "CPUSteal"} {
			if !slices.Contains(field.Values, name) {
				field.Values = append(field.Values, name)
			}
		}
		return app.Save(collection)
	}, nil)
}
