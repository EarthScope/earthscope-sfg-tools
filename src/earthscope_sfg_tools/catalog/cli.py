"""
Command line for the catalog access API.

    sfg-catalog export-posfilter --warehouse DIR --site-metadata site_metadata.json \\
        --network N --station S --campaign C [--survey ID | --start T --end T] --out DIR

Without --survey or a window, every survey of the campaign is exported to
its own directory under --out. Each export is then run with
``gnatss run <dir>/config.yaml --posfilter --no-run-all``, or, exported with
--solver, ``gnatss run <dir>/config.yaml`` for the posfilter and the array
position.
"""

import argparse
import logging
import sys
from pathlib import Path

from ..datamodels.iceberg.schemas import Scope
from ..datamodels.metadata.earthscope.site import Site
from .posfilter import SolverOptions, export_posfilter
from .reader import Level1Reader, open_catalog
from .site import PosfilterSite


def _export_posfilter(args: argparse.Namespace) -> int:
    reader = Level1Reader(open_catalog(args.warehouse, type=args.catalog, region=args.region))
    site_meta = Site.from_json(str(args.site_metadata))
    scope = Scope(args.network, args.station, args.campaign)
    out = Path(args.out)
    solver = SolverOptions(gps_sigma_limit=args.gps_sigma_limit) if args.solver else None

    if args.survey or args.start or args.end:
        runs = [(args.survey, out)]
    else:
        campaign = next((c for c in site_meta.campaigns if c.name == args.campaign), None)
        if campaign is None or not campaign.surveys:
            sys.exit(f"campaign {args.campaign} has no surveys; give --start/--end")
        runs = [(sv.id, out / sv.id) for sv in campaign.surveys]

    failed = 0
    for survey, out_dir in runs:
        try:
            site = PosfilterSite.from_metadata(
                site_meta, args.campaign, survey=survey, start=args.start, end=args.end
            )
            s = export_posfilter(
                reader, site, scope, out_dir,
                positions=args.positions, sound_speed=not args.no_sound_speed, solver=solver,
                position_sigmas=args.position_sigmas,
            )  # fmt: skip
        except ValueError as e:
            failed += 1
            print(f"{survey or 'window'}: FAILED: {e}", file=sys.stderr)
            continue
        print(
            f"{survey or 'window'}: {out_dir}  pings={s.pings} replies={s.replies} "
            f"inspva={s.inspva_good}/{s.inspva_rows} gps={s.gps_rows} "
            f"delays={s.internal_delays}" + (f"  warnings={len(s.warnings)}" if s.warnings else "")
        )
    return 1 if failed else 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="sfg-catalog", description=__doc__.split("\n\n")[0])
    sub = parser.add_subparsers(dest="command", required=True)

    p = sub.add_parser("export-posfilter", help="export GNATSS posfilter inputs and config")
    p.add_argument("--warehouse", required=True, help="warehouse directory (sql) or s3:// URI (glue)")
    p.add_argument("--catalog", default="sql", choices=["sql", "glue"])
    p.add_argument("--region", help="AWS region for --catalog glue")
    p.add_argument("--site-metadata", required=True, type=Path, help="EarthScope site_metadata.json")
    p.add_argument("--network", required=True)
    p.add_argument("--station", required=True)
    p.add_argument("--campaign", required=True)
    p.add_argument("--survey", help="survey id (default: every survey of the campaign)")
    p.add_argument("--start", help="UTC window start (ISO), instead of a survey")
    p.add_argument("--end", help="UTC window end (ISO), instead of a survey")
    p.add_argument("--positions", default="DFOP00", help="antenna position stream file type (default DFOP00)")
    p.add_argument(
        "--position-sigmas", default="logged", choices=["logged", "insstdev"],
        help="antenna sigmas: as logged, or the INS position sigmas (for DFOP00, which logs the INS position)",
    )  # fmt: skip
    p.add_argument("--no-sound-speed", action="store_true", help="skip the sound speed profile")
    p.add_argument("--solver", action="store_true", help="add a GNATSS solver section (array position)")
    p.add_argument(
        "--gps-sigma-limit", type=float, default=SolverOptions.gps_sigma_limit,
        help="solver: max antenna 3D sigma [m] (default %(default)s)",
    )  # fmt: skip
    p.add_argument("--out", required=True, help="output directory")
    p.add_argument("-v", "--verbose", action="store_true")
    p.set_defaults(func=_export_posfilter)

    args = parser.parse_args(argv)
    logging.basicConfig(level=logging.INFO if args.verbose else logging.WARNING, format="%(levelname)s %(message)s")
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
