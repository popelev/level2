#!/usr/bin/env python3
"""Copy OPC_UA_* from deploy/smoke/.env into deploy/platform/.env (VM lab helper)."""
from pathlib import Path

root = Path(__file__).resolve().parents[1]
smoke = root / "smoke" / ".env"
platform = root / "platform" / ".env"


def parse_env(path: Path) -> dict[str, str]:
    out: dict[str, str] = {}
    if not path.exists():
        return out
    for line in path.read_text(encoding="utf-8").splitlines():
        s = line.strip()
        if not s or s.startswith("#") or "=" not in s:
            continue
        k, v = s.split("=", 1)
        v = v.strip()
        if len(v) >= 2 and v[0] == v[-1] and v[0] in "\"'":
            v = v[1:-1]
        out[k.strip()] = v
    return out


def write_env(path: Path, lines: list[str]) -> None:
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> None:
    src = parse_env(smoke)
    for key in ("OPC_UA_USERNAME", "OPC_UA_PASSWORD", "PLC_OPC_ENDPOINT"):
        if key not in src:
            print(f"missing {key} in {smoke}")
            return
    raw = platform.read_text(encoding="utf-8").splitlines() if platform.exists() else []
    keys = {"OPC_UA_USERNAME", "OPC_UA_PASSWORD", "PLC_OPC_ENDPOINT", "LEVEL2_SIM_BROWSER"}
    out: list[str] = []
    seen: set[str] = set()
    for line in raw:
        if "=" in line and not line.strip().startswith("#"):
            k = line.split("=", 1)[0].strip()
            if k in keys:
                if k == "LEVEL2_SIM_BROWSER":
                    out.append("LEVEL2_SIM_BROWSER=0")
                else:
                    out.append(f"{k}={quote_env(src[k])}")
                seen.add(k)
                continue
        out.append(line)
    for k in keys:
        if k in seen:
            continue
        if k == "LEVEL2_SIM_BROWSER":
            out.append("LEVEL2_SIM_BROWSER=0")
        elif k in src:
            out.append(f"{k}={quote_env(src[k])}")
    write_env(platform, out)
    print("updated", platform)


def quote_env(val: str) -> str:
    if val == "":
        return ""
    if any(c in val for c in " #!$\"'\\"):
        return '"' + val.replace("\\", "\\\\").replace('"', '\\"') + '"'
    return val


if __name__ == "__main__":
    main()
