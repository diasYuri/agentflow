import json
import sys


def main() -> None:
    payload = json.load(sys.stdin)
    print(json.dumps({
        "message": payload["with"].get("message", ""),
        "workflow": payload["run"]["workflow"],
    }))


if __name__ == "__main__":
    main()

