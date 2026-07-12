#!/usr/bin/env python3
"""Read/write progress state for govnohub 200-item plan. JSON in .hermes/progress.json"""
import json, sys, os

REPO = "/home/ubuntu/govnohub"
PATH = os.path.join(REPO, ".hermes", "progress.json")

def read():
    with open(PATH) as f:
        return json.load(f)

def write(state):
    with open(PATH, "w") as f:
        json.dump(state, f, indent=2)

def get_next(state):
    """Return the next unstarted item number."""
    done = set(state.get("items_completed", []))
    failed = set(state.get("items_failed", []))
    in_prog = set(state.get("items_in_progress", []))
    blocked = done | failed | in_prog
    curr = state.get("current_item", 1)
    while curr in blocked:
        curr += 1
    return curr

if __name__ == "__main__":
    cmd = sys.argv[1] if len(sys.argv) > 1 else "status"
    state = read()
    if cmd == "status":
        done = len(state["items_completed"])
        failed = len(state["items_failed"])
        in_prog = len(state["items_in_progress"])
        nxt = get_next(state)
        print(json.dumps({"done": done, "failed": failed, "in_progress": in_prog,
                          "next": nxt, "last_commit": state.get("last_commit"),
                          "ci_status": state.get("ci_status")}))
    elif cmd == "start":
        n = int(sys.argv[2])
        state["items_in_progress"].append(n)
        state["current_item"] = n
        write(state)
        print(f"Started item {n}")
    elif cmd == "complete":
        n = int(sys.argv[2])
        if n in state["items_in_progress"]:
            state["items_in_progress"].remove(n)
        state["items_completed"].append(n)
        if n >= state["current_item"]:
            state["current_item"] = n + 1
        write(state)
        print(f"Completed item {n}")
    elif cmd == "fail":
        n = int(sys.argv[2])
        if n in state["items_in_progress"]:
            state["items_in_progress"].remove(n)
        state["items_failed"].append(n)
        if n >= state["current_item"]:
            state["current_item"] = n + 1
        write(state)
        print(f"Failed item {n}")
    elif cmd == "set-ci":
        state["ci_status"] = sys.argv[2]
        write(state)
        print(f"CI status: {sys.argv[2]}")
    elif cmd == "set-commit":
        state["last_commit"] = sys.argv[2]
        write(state)
        print(f"Commit: {sys.argv[2]}")
    else:
        print(f"Unknown: {cmd}")
        sys.exit(1)
