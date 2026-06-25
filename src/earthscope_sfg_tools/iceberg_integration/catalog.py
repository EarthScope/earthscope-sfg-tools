"""
Catalog factory for Apache Iceberg.

Returns a PyIceberg catalog configured for either local filesystem use (testing)
or AWS Glue (production). All table operations use the same Catalog API regardless
of which backend is chosen.
"""

import os
from pathlib import Path
from typing import Literal

from pyiceberg.catalog import Catalog
from pyiceberg.catalog.glue import GlueCatalog
from pyiceberg.catalog.sql import SqlCatalog

CatalogMode = Literal["local", "glue"]

_DEFAULT_GLUE_REGION = "us-east-2"
_DEFAULT_NAMESPACE = "sfg"


def get_catalog(
    mode: CatalogMode = "glue",
    local_warehouse: str | Path | None = None,
    glue_region: str = _DEFAULT_GLUE_REGION,
) -> Catalog:
    """
    Return a configured PyIceberg catalog.

    Args:
        mode: "glue" for production (AWS Glue metastore + S3), or "local" for a
            SQLite-backed filesystem catalog used in tests and offline work.
        local_warehouse: Root directory for the local catalog warehouse. Only used
            when mode="local". Defaults to a "warehouse" folder in the current
            working directory.
        glue_region: AWS region for the Glue catalog. Defaults to "us-east-2".

    Returns:
        A PyIceberg Catalog instance.
    """
    if mode == "local":
        warehouse_path = Path(local_warehouse or Path.cwd() / "warehouse")
        warehouse_path.mkdir(parents=True, exist_ok=True)
        return SqlCatalog(
            "sfg_local",
            **{
                "uri": f"sqlite:///{warehouse_path}/catalog.db",
                "warehouse": f"file://{warehouse_path}",
            },
        )

    if mode == "glue":
        props: dict[str, str] = {
            "region_name": glue_region,
        }
        aws_profile = os.environ.get("AWS_PROFILE", "")
        if aws_profile:
            props["profile_name"] = aws_profile
        return GlueCatalog("sfg", **props)

    raise ValueError(f"Unknown catalog mode: {mode!r}. Choose 'local' or 'glue'.")


def ensure_namespace(catalog: Catalog, namespace: str = _DEFAULT_NAMESPACE) -> None:
    """Create the namespace in the catalog if it does not already exist."""
    if (namespace,) not in catalog.list_namespaces():
        catalog.create_namespace(namespace)
