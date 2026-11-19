# Runtime providers

This directory contains standalone runtime-provider modules for the Oracle Database driver. Each module is independently versioned and exposes the token methods needed by the driver's provider registration surface without depending on a particular driver release.

OCI IAM database-token support is available in the [`oci`](./oci) module.
