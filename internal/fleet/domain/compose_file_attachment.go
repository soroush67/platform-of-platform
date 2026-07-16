package domain

// NetworkAttachment/VolumeAttachment are the junction rows behind
// migrations/0019_fleet.up.sql's compose_file_networks/
// compose_file_volumes tables - a ComposeFile references a Network/
// Volume by id, it never duplicates the catalog entry's own fields.
// Attachment applies to every service in the rendered compose file, not
// per-service - a deliberate simplification carried over from the
// ported Python product (its own compose_builder.py works the same
// way), matching that most ComposeFiles here are single-service.
type NetworkAttachment struct {
	ID            string
	ComposeFileID string
	NetworkID     string
}

type VolumeAttachment struct {
	ID            string
	ComposeFileID string
	VolumeID      string
	ContainerPath string
}
