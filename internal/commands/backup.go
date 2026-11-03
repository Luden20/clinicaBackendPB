package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/spf13/cobra"
)

// Register añade comandos propios al CLI de PocketBase.
func Register(app *pocketbase.PocketBase) {
	app.RootCmd.AddCommand(backupCmd(app))
}

// backupCmd genera un backup nativo de PocketBase (zip de pb_data) y lo guarda
// donde diga Settings → Backups: en S3 si está activado, si no en pb_data/backups.
// Pensado para correr con el servicio parado (lo usa deploy.sh antes de migrar).
func backupCmd(app *pocketbase.PocketBase) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:          "backup",
		Short:        "Crea un backup de PocketBase (sube a S3 si está configurado)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				name = "pb_backup_" + time.Now().UTC().Format("20060102150405") + ".zip"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			if err := app.CreateBackup(ctx, name); err != nil {
				return fmt.Errorf("backup %s: %w", name, err)
			}
			dest := "pb_data/backups (local)"
			if s3 := app.Settings().Backups.S3; s3.Enabled {
				dest = fmt.Sprintf("s3://%s", s3.Bucket)
			}
			fmt.Printf("Backup %s creado en %s\n", name, dest)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "nombre del zip (por defecto pb_backup_<fecha>.zip)")
	return cmd
}
