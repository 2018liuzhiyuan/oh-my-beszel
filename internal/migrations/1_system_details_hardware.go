package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Adds hardware-inventory fields to the system_details collection for hubs
// upgrading from a version that predates them. Fresh installs already get
// these fields from the collections snapshot.
func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("system_details")
		if err != nil {
			return err
		}
		addText := func(id, name string) {
			if collection.Fields.GetByName(name) == nil {
				collection.Fields.Add(&core.TextField{Id: id, Name: name})
			}
		}
		addNumber := func(id, name string) {
			if collection.Fields.GetByName(name) == nil {
				collection.Fields.Add(&core.NumberField{Id: id, Name: name, OnlyInt: true})
			}
		}
		addText("text_hw_ipaddrs", "ip_addrs")
		addNumber("number_hw_niccount", "nic_count")
		addNumber("number_hw_nicspeed", "nic_speed_mbps")
		addText("text_hw_bios", "bios_version")
		addText("text_hw_bmc", "bmc_version")
		addText("text_hw_sel", "sel_entries")
		if err := app.Save(collection); err != nil {
			return err
		}

		// allow the GpuMemoryFree alert type on the alerts collection
		alertsCol, err := app.FindCollectionByNameOrId("alerts")
		if err != nil {
			return err
		}
		nameField := alertsCol.Fields.GetByName("name")
		if sel, ok := nameField.(*core.SelectField); ok {
			found := false
			for _, v := range sel.Values {
				if v == "GpuMemoryFree" {
					found = true
					break
				}
			}
			if !found {
				sel.Values = append(sel.Values, "GpuMemoryFree")
				if err := app.Save(alertsCol); err != nil {
					return err
				}
			}
		}
		return nil
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("system_details")
		if err != nil {
			return err
		}
		for _, name := range []string{"ip_addrs", "nic_count", "nic_speed_mbps", "bios_version", "bmc_version", "sel_entries"} {
			collection.Fields.RemoveByName(name)
		}
		return app.Save(collection)
	})
}
