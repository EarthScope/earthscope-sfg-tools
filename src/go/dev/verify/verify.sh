#!/usr/bin/env bash
# verify.sh: end-to-end check of the sfg binary on the repo's raw fixtures.
#
#   dev/verify/verify.sh local   # a fresh temporary local warehouse
#   dev/verify/verify.sh glue    # AWS Glue, development databases only
#
# Prints sorted lines to stdout (progress and sfg's logs go to stderr):
#   count<TAB><table><TAB><stream_edid><TAB><rows>   every Level 0 table with rows
#   count-files<TAB>...                              the same, counted in the CSVs of raw -> files
#   rinex<TAB><stream_edid><TAB><sha256>             RINEX body, without its "PGM / RUN BY / DATE" line
#   rinex-iceberg<TAB><stream_edid><TAB><sha256>     the same, from the ingested warehouse (equal to the rinex line)
#   check<TAB><name><TAB>ok|FAIL
# and exits non-zero if any check fails. Later changes are verified by
# diffing this output with dev/verify/baseline.txt, and the glue output with
# the local output.
#
# glue mode only ever touches the development databases: it needs
# SFG_DEV_WAREHOUSE (an S3 prefix for development data, never production's)
# and always passes --database-prefix dev_ (dev_gnssa_l0, dev_gnssa_l1,
# dev_gnssa_l2). AWS credentials and region come from the default chain
# (AWS_REGION, the profile).
set -euo pipefail

mode="${1:-}"
case "$mode" in
local | glue) ;;
*)
	echo "usage: $0 local|glue" >&2
	exit 2
	;;
esac

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
go_dir="$(cd "$here/../.." && pwd)"
data="$(cd "$go_dir/../.." && pwd)/tests/data"
scope="verify/VRFY/2026_A_0001"
dev_prefix="dev_"
fixtures=(
	"$data/test_20260331_174553_00030_NOV770.raw"
	"$data/test_gcc1_20250821_205746_00062_NOV000.bin"
	"$data/test_gcc1_20250822_DFOP00.raw"
)
# Every table of gnssa_l0 (datamodels.GNSSA_L0), by its bare name.
tables=(
	travel_times inspva insstdev gnss_observations attitude_ahrs sound_speed
	gnss_antenna_positions gnss_satellites_in_view gnss_dop gnss_course_speed
	transceiver_interrogations transceiver_ranges transceiver_settings
	gnssa_community_standard
)

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
lines="$tmp/lines"
: >"$lines"
failed=0

say() { echo "verify: $*" >&2; }

# check NAME STATUS records a check; STATUS is 0 for ok.
check() {
	local result=ok
	if [ "$2" != 0 ]; then
		result=FAIL
		failed=1
	fi
	printf 'check\t%s\t%s\n' "$1" "$result" >>"$lines"
}

sha256() { shasum -a 256 | cut -d' ' -f1; }

for f in "${fixtures[@]}"; do
	[ -f "$f" ] || {
		echo "verify: missing fixture $f" >&2
		exit 2
	}
done

# The warehouse flags every iceberg command takes.
case "$mode" in
local)
	wh=(--catalog local --warehouse "$tmp/warehouse")
	;;
glue)
	: "${SFG_DEV_WAREHOUSE:?set SFG_DEV_WAREHOUSE to the S3 prefix for development data}"
	case "$SFG_DEV_WAREHOUSE" in
	s3://*) ;;
	*)
		echo "verify: SFG_DEV_WAREHOUSE must be an s3:// prefix" >&2
		exit 2
		;;
	esac
	case "$SFG_DEV_WAREHOUSE" in
	*gnssa-test*)
		echo "verify: SFG_DEV_WAREHOUSE looks like production; refusing" >&2
		exit 2
		;;
	esac
	wh=(--catalog glue --warehouse "$SFG_DEV_WAREHOUSE" --database-prefix "$dev_prefix")
	;;
esac

# 1. Build sfg.
say "building sfg"
sfg="$tmp/sfg"
rc=0
(cd "$go_dir" && go build -o "$sfg" ./cmd/sfg) >"$tmp/go-build.log" 2>&1 || rc=$?
check build "$rc"
if [ "$rc" != 0 ]; then
	cat "$tmp/go-build.log" >&2
	sort "$lines"
	exit 1
fi

# definitions: the datamodels declarations are consistent and the README
# diagram generated from them is current.
rc=0
(cd "$go_dir" && go run ./dev/gen_readme -check) >"$tmp/definitions.log" 2>&1 || rc=$?
if [ "$rc" != 0 ]; then
	cat "$tmp/definitions.log" >&2
fi
check definitions "$rc"

# run NAME CMD... runs sfg, keeping its log in $tmp/NAME.log, and sets rc.
run() {
	local name="$1"
	shift
	rc=0
	"$sfg" "$@" >"$tmp/$name.log" 2>&1 || rc=$?
	if [ "$rc" != 0 ]; then
		say "$name exited $rc:"
		tail -n 20 "$tmp/$name.log" >&2
	fi
}

# 2-3. Build the catalog (Glue: the dev_ databases).
say "iceberg build ($mode)"
run build iceberg build "${wh[@]}"
check iceberg-build "$rc"

# ingest runs iceberg ingest of the three fixtures.
ingest() {
	run "$1" iceberg ingest "${wh[@]}" --scope "$scope" --result "$tmp/$1.json" "${fixtures[@]}"
}

# counts DIR prints "table<TAB>stream_edid<TAB>rows" for every table with
# rows, exporting each table as csv into DIR. Tables that fail to export
# are listed on stderr and fail the check.
counts() {
	local dir="$1" t
	mkdir -p "$dir"
	for t in "${tables[@]}"; do
		rc=0
		"$sfg" iceberg export "${wh[@]}" --scope "$scope" --table "gnssa_l0.$t" \
			--format csv --out "$dir" >"$tmp/export-$t.log" 2>&1 || rc=$?
		# A table with no rows for the scope exports nothing and exits 1
		# ("selection matched no rows"): it has no counts, which is not a failure.
		if [ "$rc" = 1 ] && grep -q "selection matched no rows" "$tmp/export-$t.log"; then
			continue
		fi
		if [ "$rc" != 0 ]; then
			say "export $t exited $rc:"
			tail -n 5 "$tmp/export-$t.log" >&2
			return 1
		fi
		# Each table is one file, <dir>/<database>/<table>/part-00000.csv.
		# The stream_edid column is found from each header; streams are
		# counted in the rows below it. A table with no rows has no files.
		csvs=("$dir/gnssa_l0/$t"/*.csv)
		[ -e "${csvs[0]}" ] || continue
		awk -F, -v table="$t" '
			FNR == 1 { col = 0; for (i = 1; i <= NF; i++) if ($i == "stream_edid") col = i; next }
			col { n[$col]++ }
			END { for (s in n) printf "%s\t%s\t%d\n", table, s, n[s] }
		' "${csvs[@]}"
	done
}

# 4-5. Ingest, export and count.
say "iceberg ingest ($mode)"
ingest ingest
check ingest "$rc"
say "counting rows"
rc=0
counts "$tmp/export1" >"$tmp/counts1" || rc=$?
check export "$rc"
LC_ALL=C sort "$tmp/counts1" | awk -F'\t' '{ printf "count\t%s\t%s\t%s\n", $1, $2, $3 }' >>"$lines"

# fixtures-yield: the baseline must not be blind to a file type. DFOP00 and
# NOV770 each give rows. The NOV000 fixture is expected to give none: every
# record in it was logged before the receiver had a time fix (header time
# status UNKNOWN, GPS week 0), and the NOV000 parser skips such records on
# purpose (pkg/fileparse/novatel.go). It must still parse with status ok.
rows_of() { awk -F'\t' -v t="$1" '$2 ~ ("\\." t "\\.") { n += $3 } END { print n + 0 }' "$tmp/counts1"; }
yield_rc=0
[ "$(rows_of DFOP00)" -gt 0 ] || yield_rc=1
[ "$(rows_of NOV770)" -gt 0 ] || yield_rc=1
[ "$(rows_of NOV000)" -eq 0 ] || yield_rc=1
# The result file lists each input as "type": ... followed by "status": ...
awk '/"type": "NOV000"/ { getline; if ($0 ~ /"status": "ok"/) ok = 1 } END { exit !ok }' "$tmp/ingest.json" || yield_rc=1
check fixtures-yield "$yield_rc"

# 6. Ingest again: replacing must leave the counts unchanged.
say "iceberg ingest again ($mode)"
ingest reingest
check reingest-run "$rc"
rc=0
counts "$tmp/export2" >"$tmp/counts2" || rc=$?
if [ "$rc" = 0 ] && cmp -s <(LC_ALL=C sort "$tmp/counts1") <(LC_ALL=C sort "$tmp/counts2"); then
	check reingest 0
else
	check reingest 1
fi

# bad-log-source: a log source no table lists is a usage error (exit 2), found
# before any file is read, so nothing is written (the counts are unchanged).
rc=0
"$sfg" iceberg ingest "${wh[@]}" --scope "$scope" --log-source TCVR00.NOPE \
	--result "$tmp/bad-log-source.json" "${fixtures[2]}" >"$tmp/bad-log-source.log" 2>&1 || rc=$?
bad_rc=0
[ "$rc" = 2 ] || bad_rc=1
rc=0
counts "$tmp/export3" >"$tmp/counts3" || rc=$?
[ "$rc" = 0 ] && cmp -s <(LC_ALL=C sort "$tmp/counts1") <(LC_ALL=C sort "$tmp/counts3") || bad_rc=1
check bad-log-source "$bad_rc"

# load-creates-nothing: reading a warehouse that doesn't exist fails and leaves
# no catalog behind (local mode only; a Glue catalog is never created by a read).
if [ "$mode" = local ]; then
	empty="$tmp/empty-warehouse"
	mkdir -p "$empty"
	rc=0
	"$sfg" iceberg export --catalog local --warehouse "$empty" --scope "$scope" --table "gnssa_l0.inspva" \
		--format csv --out "$tmp/empty-out" >"$tmp/load-creates-nothing.log" 2>&1 || rc=$?
	lc_rc=0
	[ "$rc" != 0 ] || lc_rc=1
	[ ! -e "$empty/catalog.db" ] || lc_rc=1
	check load-creates-nothing "$lc_rc"
else
	check load-creates-nothing 0
fi

# raw -> files: each fixture to CSV under $tmp/files/<database>/<table>/, one
# file per raw file and table. The per-table, per-stream counts of those CSVs
# must equal the count lines (count-files lines are compared with them in
# baseline.txt). A fixture that fails to convert fails the run.
say "raw to files"
types=(NOV770 NOV000 DFOP00)
for i in 0 1 2; do
	run "files-${types[$i]}" convert --source raw --source-type "${types[$i]}" --scope "$scope" \
		--sink files --table-format csv --sink-uri "$tmp/files" --source-uri "${fixtures[$i]}"
	[ "$rc" = 0 ] || failed=1
done
for t in "${tables[@]}"; do
	csvs=("$tmp/files/gnssa_l0/$t"/*.csv)
	[ -e "${csvs[0]}" ] || continue
	awk -F, -v table="$t" '
		FNR == 1 { col = 0; for (i = 1; i <= NF; i++) if ($i == "stream_edid") col = i; next }
		col { n[$col]++ }
		END { for (s in n) printf "count-files\t%s\t%s\t%d\n", table, s, n[s] }
	' "${csvs[@]}" >>"$lines"
done

# files-to-iceberg: raw -> Parquet files, then those files -> a fresh local
# warehouse (in either mode: it never touches Glue), then export. The per-stream
# counts must equal the count lines, which also exercises the files source.
say "files to iceberg"
fi_rc=0
for i in 0 1 2; do
	run "pq-${types[$i]}" convert --source raw --source-type "${types[$i]}" --scope "$scope" \
		--sink files --table-format parquet --sink-uri "$tmp/pq" --source-uri "${fixtures[$i]}"
	[ "$rc" = 0 ] || fi_rc=1
done
run files-to-iceberg convert --source files --source-uri "$tmp/pq" --table-format parquet --scope "$scope" \
	--sink iceberg --sink-catalog local --sink-uri "$tmp/warehouse-files"
[ "$rc" = 0 ] || fi_rc=1
# Only the tables the files hold were created in that warehouse.
wh_saved=("${wh[@]}")
tables_saved=("${tables[@]}")
wh=(--catalog local --warehouse "$tmp/warehouse-files")
tables=()
for t in "${tables_saved[@]}"; do
	[ -d "$tmp/pq/gnssa_l0/$t" ] && tables+=("$t")
done
rc=0
counts "$tmp/export-files" >"$tmp/counts-files" || rc=$?
wh=("${wh_saved[@]}")
tables=("${tables_saved[@]}")
[ "$rc" = 0 ] || fi_rc=1
cmp -s <(LC_ALL=C sort "$tmp/counts1") <(LC_ALL=C sort "$tmp/counts-files") || fi_rc=1
[ -s "$tmp/counts-files" ] || fi_rc=1
check files-to-iceberg "$fi_rc"

# partial: a file that fails to parse beside one that parses is a partial run
# (exit 3): the good file's CSV is written and the bad file is reported failed.
say "partial run"
mkdir -p "$tmp/partial"
# The DFOP00 parser skips lines that are not JSON, so garbage must be unreadable
# as lines: one line longer than the reader's 16 MiB limit.
head -c 17000000 /dev/zero | tr '\0' x >"$tmp/partial/bad_DFOP00.raw"
rc=0
"$sfg" convert --source raw --source-type DFOP00 --scope "$scope" --sink files --table-format csv \
	--sink-uri "$tmp/partial/out" --result "$tmp/partial/result.json" \
	--source-uri "${fixtures[2]}" --source-uri "$tmp/partial/bad_DFOP00.raw" \
	>"$tmp/partial/run.log" 2>&1 || rc=$?
partial_rc=0
[ "$rc" = 3 ] || partial_rc=1
# The result lists each input as "path": ..., "type": ..., "status": ...
input_status() { awk -v name="$1" -v want="$2" '
	index($0, "\"path\": ") && index($0, name "\"") { getline; getline; if (index($0, "\"status\": \"" want "\"")) ok = 1 }
	END { exit !ok }' "$tmp/partial/result.json"; }
input_status bad_DFOP00.raw failed || partial_rc=1
input_status test_gcc1_20250822_DFOP00.raw ok || partial_rc=1
[ -f "$tmp/partial/out/gnssa_l0/inspva/test_gcc1_20250822_DFOP00.csv" ] || partial_rc=1
check partial "$partial_rc"

# 7. RINEX from the raw NOV770 fixture, once per NOV770 gnss_observations
# stream, found in the counts. The checksum leaves out the one header line
# that holds the run date.
nov770="$data/test_20260331_174553_00030_NOV770.raw"
settings="$here/rinex_settings.json"
rinex_rc=0
streams="$(awk -F'\t' '$1 == "gnss_observations" && $2 ~ /\.NOV770\./ { print $2 }' "$tmp/counts1" | LC_ALL=C sort)"
[ -n "$streams" ] || rinex_rc=1
for stream in $streams; do
	log_source="${stream##*.}"
	say "sfg rinex $log_source"
	out="$tmp/rinex-$log_source"
	rc=0
	"$sfg" rinex --type NOV770 --log-source "$log_source" --settings "$settings" \
		--out "$out" "$nov770" >"$tmp/rinex-$log_source.log" 2>&1 || rc=$?
	if [ "$rc" != 0 ]; then
		say "sfg rinex $log_source exited $rc:"
		tail -n 5 "$tmp/rinex-$log_source.log" >&2
		rinex_rc=1
		continue
	fi
	sum="$(find "$out" -type f -name '*.rnx' | LC_ALL=C sort | while read -r f; do
		grep -v 'PGM / RUN BY / DATE' "$f"
	done | sha256)"
	printf 'rinex\t%s\t%s\n' "$stream" "$sum" >>"$lines"
done
check rinex "$rinex_rc"

# 8. RINEX from the ingested warehouse: the same per-stream files, read from
# the catalog the fixtures were ingested into. Their digests must equal the
# rinex lines above (compared in baseline.txt, and here).
ri_rc=0
for stream in $streams; do
	log_source="${stream##*.}"
	say "sfg rinex --source iceberg $log_source"
	out="$tmp/rinex-iceberg-$log_source"
	rc=0
	"$sfg" rinex --source iceberg "${wh[@]}" --scope "$scope" --type NOV770 --log-source "$log_source" \
		--settings "$settings" --out "$out" >"$tmp/rinex-iceberg-$log_source.log" 2>&1 || rc=$?
	if [ "$rc" != 0 ]; then
		say "sfg rinex --source iceberg $log_source exited $rc:"
		tail -n 5 "$tmp/rinex-iceberg-$log_source.log" >&2
		ri_rc=1
		continue
	fi
	sum="$(find "$out" -type f -name '*.rnx' | LC_ALL=C sort | while read -r f; do
		grep -v 'PGM / RUN BY / DATE' "$f"
	done | sha256)"
	printf 'rinex-iceberg\t%s\t%s\n' "$stream" "$sum" >>"$lines"
	[ "$sum" = "$(grep -F "rinex	$stream	" "$lines" | cut -f3)" ] || ri_rc=1
done
[ -n "$streams" ] || ri_rc=1
check rinex-iceberg "$ri_rc"

# rinex-two-streams: the scope's gnss_observations hold two NOV770 streams
# (two antennas), and a RINEX file is one antenna's: without --log-source the
# command is a usage error (exit 2) naming both streams, and writes nothing.
rc=0
"$sfg" rinex --source iceberg "${wh[@]}" --scope "$scope" --settings "$settings" \
	--out "$tmp/rinex-two" >"$tmp/rinex-two.log" 2>&1 || rc=$?
two_rc=0
[ "$rc" = 2 ] || two_rc=1
n_streams=0
for stream in $streams; do
	n_streams=$((n_streams + 1))
	grep -qF "$stream" "$tmp/rinex-two.log" || two_rc=1
done
[ "$n_streams" -ge 2 ] || two_rc=1
[ -z "$(ls -A "$tmp/rinex-two" 2>/dev/null)" ] || two_rc=1
check rinex-two-streams "$two_rc"

# rinex-bad-log-source: a log source that exists but is not a gnss_observations
# stream (an INS log) is a usage error (exit 2), not a failed run.
rc=0
"$sfg" rinex --source iceberg "${wh[@]}" --scope "$scope" --type NOV000 --log-source INSPVAA \
	--settings "$settings" --out "$tmp/rinex-bad" >"$tmp/rinex-bad.log" 2>&1 || rc=$?
bad_rc=0
[ "$rc" = 2 ] || bad_rc=1
rc=0
"$sfg" rinex --source iceberg "${wh[@]}" --scope "$scope" --type DFOP00 --log-source INSPVAA \
	--settings "$settings" --out "$tmp/rinex-bad" >"$tmp/rinex-bad2.log" 2>&1 || rc=$?
[ "$rc" = 2 ] || bad_rc=1
check rinex-bad-log-source "$bad_rc"

LC_ALL=C sort "$lines"
exit "$failed"
