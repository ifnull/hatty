"""Capture live subscribe_entities frames as a test fixture."""
import asyncio, json, os, sys, time, websockets

ENTS = ["sensor.airspace_nearest_aircraft","sensor.airspace_flag_military",
        "sensor.airspace_flag_interesting","sensor.airspace_flag_emergency",
        "sensor.airspace_aircraft_count","sensor.airspace_drone_count",
        "binary_sensor.airspace_alert_military_close",
        "sensor.airspace_receiver_rx_1090_message_rate",
        "sensor.st_00128663_feels_like","sensor.st_00128663_humidity",
        "sensor.st_00128663_wind_gust","sensor.st_00128663_wind_speed_average",
        "sensor.st_00128663_wind_direction_average","sensor.st_00128663_precipitation",
        "sensor.st_00128663_battery","event.st_00128663_lightning_strike",
        "sensor.awair_element_128797_score","sensor.awair_element_128797_temperature",
        "sensor.awair_element_128797_carbon_dioxide","sensor.awair_element_128797_pm2_5"]

async def main(secs, out):
    tok = os.environ["HATTY_HA_TOKEN"]
    async with websockets.connect("ws://ha.home.arpa:8123/api/websocket", max_size=None) as ws:
        await ws.recv()
        await ws.send(json.dumps({"type":"auth","access_token":tok}))
        assert json.loads(await ws.recv())["type"] == "auth_ok"
        await ws.send(json.dumps({"id":1,"type":"subscribe_entities","entity_ids":ENTS}))
        assert json.loads(await ws.recv())["success"]
        n = 0
        with open(out, "w") as fh:
            end = time.monotonic() + secs
            while time.monotonic() < end:
                try:
                    raw = await asyncio.wait_for(ws.recv(), timeout=end-time.monotonic())
                except asyncio.TimeoutError:
                    break
                m = json.loads(raw)
                if m.get("type") == "event":
                    fh.write(json.dumps({"t": round(time.time(),3), "e": m["event"]}) + "\n")
                    n += 1
        print(f"{out}: {n} frames, {os.path.getsize(out):,} B")

asyncio.run(main(int(sys.argv[1]), sys.argv[2]))
