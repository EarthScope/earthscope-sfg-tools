# Level 0 file types and log sources

Every raw (Level 0) file type, the log sources it contains, and the Level 1 table each feeds. A *log source* is one stream of records within a file. It is what a parser selects, and it becomes the last part of a row's `stream_edid` (`network:station:campaign:file_type:log_source`).

Sources:
- The parsers in `fileparse/`.
- A survey of `SFGMain2/cascadia-gorda/SLT1/2026_A_1126/raw`, the default `-data` of `main.go`, on 2026-10-29. The script decodes the GPSA framing; JSON for DFOP00; NovAtel headers for NOV770.

Log sources that the code declares but the SLT1 campaign doesn't contain are marked as such. Names for unparsed log sources are proposals, taken from the message's own identifier.

## Matrix

Rows are file types and columns are log sources. A cell holds the Level 1 table the log source feeds, `·` if the file type contains that log source but nothing parses it, and is empty if the file type doesn't have that log source.

Table codes: **PVA** inspva · **STD** insstdev · **OBS** gnss_observations · **POS** gnss_antenna_positions · **ATT** attitude_ahrs · **TT** travel_times · **SV** sound_speed · **SAT** gnss_satellites_in_view · **DOP** gnss_dop · **COG** gnss_course_speed · **INT** transceiver_interrogations · **RNG** transceiver_ranges · **SET** transceiver_settings.

| File type | INSPVAA | INSSTDEVA | RANGEA | DUALANTENNAHEADINGA | RANGECMPB | RANGECMPB_1 | RANGECMP5B | RANGECMP5B_1 | BESTPOSB | RAWEPHEMB | GLOEPHEMERIS | DFOP00 | NOV_INS | NOV_HEADING | NOV_RANGE | QCPIN | AHRS00 | GGA | GSA | GSV | VTG | CTD | SEABIRD | TGT | TRK | TMS | SENS | STATUS | NET | INTERROGATE | RANGE_RESULT | TCVR_CONFIG | TCVR_INFO |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| NOV000 | PVA | STD | OBS | · | | | | | | | | | | | | | | | | | | | | | | | | | | | | | |
| NOV770 | | | | | OBS | OBS | OBS | OBS | · | · | · | | | | | | | | | | | | | | | | | | | | | | |
| DFOP00 | | | | | | | | | | | | TT ATT POS | · | · | · | | | | | | | | | | | | | | | | | | |
| QCPIN | | | OBS | | | | | | | | | | | | | TT ATT POS | | | | | | | | | | | | | | | | | |
| AHRS00 | | | | | | | | | | | | | | | | | ATT | | | | | | | | | | | | | | | | |
| GNSS00 | | | | | | | | | | | | | | | | | | POS | DOP | SAT | COG | | | | | | | | | | | | |
| CTD | | | | | | | | | | | | | | | | | | | | | | SV | | | | | | | | | | | |
| SEABIRD | | | | | | | | | | | | | | | | | | | | | | | SV | | | | | | | | | | |
| INOUT0 | | | | | | | | | | | | | | | | | | | | | | | | · | · | · | · | · | · | | | | |
| TCVR00 | | | | | | | | | | | | | | | | | | | | | | | | | | | | | | INT | RNG | SET | SET |

## By file type

### NOV000 — NovAtel ASCII logs in GPSA packets (`*NOV000.bin`)

| Log source | Records | Table | Notes |
|---|---|---|---|
| INSPVAA | INS position, velocity, attitude | inspva | ~256k per 4 files |
| INSSTDEVA | INS solution std devs | insstdev | |
| RANGEA | GNSS observations | gnss_observations | |
| DUALANTENNAHEADINGA | dual-antenna heading/pitch | — | **unparsed**; ~63k per 4 files. Candidate for attitude/heading |

Long logs span several GPSA packets. The payloads that start mid-message (`,0.`, `-12…`) are continuation chunks, not separate log sources.

### NOV770 — NovAtel binary logs (`*NOV770.raw`)

| Log source | Records | Table | Notes |
|---|---|---|---|
| RANGECMPB, RANGECMPB_1 | compressed range (ID 140), primary / second antenna | gnss_observations | declared by the parser; none in SLT1 2026 |
| RANGECMP5B, RANGECMP5B_1 | compressed range (ID 2537), primary / second antenna | gnss_observations | ~198k per 3 files |
| BESTPOSB | best position (ID 42) | — | **unparsed**; ~198k per 3 files, as many as RANGECMP5B. Candidate for gnss_antenna_positions |
| RAWEPHEMB | GPS raw ephemeris (ID 41) | — | **unparsed**; navigation data, no Level 1 table |
| GLOEPHEMERIS | GLONASS ephemeris (ID 723) | — | **unparsed**; navigation data |

The `_1` suffix is the antenna from each message's measurement source; see `datamodels.LogSource`. The survey script also found about a dozen single matches of the NovAtel sync bytes inside payloads; those are false positives, not log sources.

The raw folder also holds RINEX 3.04 conversions of one NOV770 file (`.26O/.26N/.26G/.26L/.26F/.26I/.26Q`, plus a `Summary_*.txt`) in a subfolder. Those are converter output, not Level 0 data.

### DFOP00 — Sonardyne SV3 events, JSON lines (`*DFOP00.raw`)

| Log source | Records | Table | Notes |
|---|---|---|---|
| DFOP00 | interrogation/range events with their AHRS and GNSS blocks | travel_times, attitude_ahrs, gnss_antenna_positions | name frozen (equals the file type). The GNSS block positions equal INSPVA exactly |
| NOV_INS | NovAtel INS block on each event | — | **unparsed**; duplicates NOV000 INSPVAA at event times |
| NOV_HEADING | NovAtel heading block | — | **unparsed**; duplicates NOV000 DUALANTENNAHEADINGA at event times |
| NOV_RANGE | NovAtel range block | — | **unparsed** in DFOP00 (QCPIN parses the same block as `RANGEA`) |

In SLT1 2026: 242 events (121 interrogations, 121 ranges). In one file the GNSS block is the string `ERR3` instead of a fix.

### QCPIN — SV3 QC file: events plus NovAtel blocks

| Log source | Records | Table | Notes |
|---|---|---|---|
| QCPIN | SV3 events | travel_times, attitude_ahrs, gnss_antenna_positions | name frozen (equals the file type); opt-in |
| RANGEA | NOV_RANGE blocks | gnss_observations | |

Not present in SLT1 2026.

### AHRS00 — Xsens MTData2 in GPSA packets (`*AHRS00.bin`)

| Log source | Records | Table |
|---|---|---|
| AHRS00 | attitude, acceleration, magnetic heading | attitude_ahrs |

About 1.0M packets per 4 files, all binary Xsens frames (GPSA message ID 990).

### GNSS00 — SV3 GNSS receiver NMEA in GPSA packets (`*GNSS00.bin`)

| Log source | Records | Table | Notes |
|---|---|---|---|
| GGA | position fix | gnss_antenna_positions | opt-in; very sparse (3 in a 2 h file) |
| GSA | fix type, DOPs and satellites used | gnss_dop | the GGA parser also uses its PDOP; GSA with no DOP (no fix) are skipped |
| GSV | satellites in view | gnss_satellites_in_view | one row per satellite per GSV group, at the group's first packet time |
| VTG | course and speed over ground | gnss_course_speed | in the logged units; VTG with neither course nor speed are skipped |

All GPSA message ID 991. The 2 h file in SLT1 2026 has 14,875 sentences.

### CTD / SEABIRD — sound speed casts

| File type | Log source | Table |
|---|---|---|
| CTD (`*CTD*_fit`) | CTD | sound_speed |
| SEABIRD (`*.cnv`) | SEABIRD | sound_speed |

Not present in SLT1 2026.

### INOUT0 — SV3 command console, text in GPSA packets (`*INOUT0.bin`) — **no parser**

Commands to the SV3 (bare, e.g. `TGT`) and its replies (prefixed `>`). 419 packets in 5 files; GPSA message ID 990. The log sources below are proposed groupings of the command families.

| Log source (proposed) | Commands | Content | Possible use |
|---|---|---|---|
| TGT | `TGT` | target transponder configuration: unit, TAT, address, TX/RX channels (`TGT:U0042D9;TAT200;AD5209;TXCI4805;RXIR5209`) | the TAT the SV3 was configured with; cross-check against site metadata |
| TRK | `TRK` | tracking sequence configuration and start/stop | survey/tracking state timeline |
| TMS | `TMS` | clock sync: local time, sync count, common time (`TMS:LT701.085;SC42,CT1780430640.724;DEF_SYNC`) | time-quality record |
| SENS | `SENS` | sensor snapshot: heading/pitch/roll, GNSS and INS status, temperature | status timeline |
| STATUS | `PS`, `DEV`, `RST` | system and device status, resets | status timeline |
| NET | `NET` | network/access point configuration | none; **contains a Wi-Fi SSID and passphrase in plain text** |

### TCVR00 — acoustic transceiver dialogue, text in GPSA packets (`*TCVR00.bin`)

Requests to the transceiver (prefixed `<`) and its replies (`>`). This is the raw exchange behind each DFOP00 interrogation and range event. 1,200 packets in 5 files; GPSA message ID 990.

| Log source | Messages | Content | Table |
|---|---|---|---|
| INTERROGATE | `IR`, `IT`, `TRG` | interrogation setup and trigger: target address, receive window, transmit channel, delay (`IR:…,IR5209;RG0;W3600;LG26`, `IT:…,CI4805;D130`) | transceiver_interrogations: one row per trigger |
| RANGE_RESULT | `BR` | per-ping detection: range, cross-correlation, SNR, level, telemetry, Doppler (`BR:…,IR5209;R0[XC21,SNR0,DBV0,TEL;NONE,DOP0.00]`) | transceiver_ranges: one row per transponder per ping, no-replies included (`detected` false). R is microseconds; it equals DFOP00's range for the same reply |
| TCVR_CONFIG | `CS`, `PORT` | transceiver settings: TAT, block, windows, gains (`CS:2010,TAT640,BLK100,RXW3600,…`), serial ports | transceiver_settings, one row per key |
| TCVR_INFO | `US`, `FS`, `DIAG` | unit serial and type, firmware, diagnostics | transceiver_settings, one row per key |

**SLT1 2026 cross-checks:**
- All 121 `BR` results have range `R0`, i.e. no reply. That matches the 121 no-reply DFOP00 events found earlier for this campaign.
- For transponder 5209, INOUT0's `TGT` says **TAT200** while TCVR00's `CS` says **TAT640**. That's worth checking against the site metadata and the no-reply result.
