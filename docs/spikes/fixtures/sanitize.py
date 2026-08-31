"""
Sanitise a captured frame trace so it can live in a public repo.

Preserves everything the tests need -- frame count, inter-frame timing, payload
sizes, diff structure, list lengths, value conditions, type shapes -- while
removing anything that could locate the home or identify real aircraft.

Location-inferring fields (distance_nm + bearing_deg + hex) would, against
public ADS-B history, triangulate the watchpoint precisely. See decision D27.
"""
import hashlib, json, sys

BRG_ROT = 137.0     # fixed rotation, applied to every bearing
DIST_K  = 1.1734    # fixed scale, applied to every distance
SALT    = b"hatty-fixture-v1"

def pseudo(v, n, alphabet="ABCDEFGHJKLMNPQRSTUVWXYZ0123456789"):
    if not isinstance(v, str) or not v.strip():
        return v
    h = hashlib.sha256(SALT + v.strip().encode()).digest()
    return "".join(alphabet[b % len(alphabet)] for b in h[:n])

def scrub(o):
    if isinstance(o, list):
        return [scrub(x) for x in o]
    if not isinstance(o, dict):
        return o
    out = {}
    for k, v in o.items():
        if k == "bearing_deg" and isinstance(v, (int, float)):
            out[k] = round((v + BRG_ROT) % 360, 1)
        elif k in ("distance_nm", "distance_to", "predicted_closest_approach_nm"):
            if isinstance(v, (int, float)):
                out[k] = round(v * DIST_K, 2)
            elif isinstance(v, dict):
                out[k] = {kk: (round(vv * DIST_K, 2) if isinstance(vv, (int, float)) else vv)
                          for kk, vv in v.items()}
            else:
                out[k] = scrub(v)
        elif k == "bearing_to" and isinstance(v, dict):
            out[k] = {kk: (round((vv + BRG_ROT) % 360, 1) if isinstance(vv, (int, float)) else vv)
                      for kk, vv in v.items()}
        elif k == "hex":
            out[k] = pseudo(v, 6, "0123456789ABCDEF").lower()
        elif k in ("flight", "callsign"):
            out[k] = pseudo(v, 7)
        elif k in ("registration", "reg"):
            out[k] = v if not isinstance(v, str) else "N" + pseudo(v, 5, "0123456789")
        elif k in ("ownop", "owner", "operator_id", "track_id"):
            out[k] = pseudo(v, 8)
        elif k in ("lat", "lon", "operator_lat", "operator_lon", "seen_by"):
            continue                       # drop outright
        else:
            out[k] = scrub(v)
    return out

src, dst, limit = sys.argv[1], sys.argv[2], int(sys.argv[3])
n = 0
t0 = None
with open(src) as fi, open(dst, "w") as fo:
    for line in fi:
        if n >= limit:
            break
        rec = json.loads(line)
        if t0 is None:
            t0 = rec["t"]
        fo.write(json.dumps({"dt": round(rec["t"] - t0, 3), "e": scrub(rec["e"])},
                            separators=(",", ":")) + "\n")
        n += 1
print(f"{dst}: {n} frames")
