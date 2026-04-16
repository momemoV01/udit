# udit Cookbook

Practical workflow recipes for common Unity automation scenarios.
Each recipe shows a goal, the command sequence, and tips for variations.

> **Prerequisites for all recipes:** Unity Editor is running with the udit Connector package installed.
> Commands assume default port (8590). Use `--port N` if you changed it.

---

## 1. CI Smoke Test

**Goal:** Verify that scripts compile and tests pass before merging.

```bash
# 1. Trigger a recompile and wait for it to finish
udit editor refresh --compile

# 2. Check for compile errors
ERRORS=$(udit console --type error --json | jq '.data.count')
if [ "$ERRORS" -gt 0 ]; then
  echo "Compile errors found:"
  udit console --type error
  exit 1
fi

# 3. Run EditMode tests with JUnit output for CI
udit test run --mode EditMode --output test-results.xml

echo "Smoke test passed."
```

**Variations:**
- Add `--mode PlayMode` for play-mode tests (requires a running game loop).
- Use `udit project preflight --json` instead of step 2 for a broader health check (compile state + build settings + missing scripts).

---

## 2. Prefab Batch Edit

**Goal:** Update a component field across all instances of a prefab in the current scene.

```bash
PREFAB="Assets/Prefabs/Enemy.prefab"
COMPONENT="Rigidbody"
FIELD="m_Mass"
VALUE="5.5"

# 1. Find all scene instances of the prefab
INSTANCES=$(udit prefab find-instances "$PREFAB" --json \
  | jq -r '.data.instances[].id')

# 2. Wrap all edits in one Undo group
udit tx begin --name "Batch set $FIELD"

# 3. Set the field on each instance
for ID in $INSTANCES; do
  udit component set "$ID" "$COMPONENT" "$FIELD" "$VALUE"
  echo "  updated $ID"
done

# 4. Commit the transaction
udit tx commit --name "Batch set $FIELD"

echo "Done. Updated $(echo "$INSTANCES" | wc -l) instances."
```

**Tips:**
- Add `--dry-run` to the `component set` call first to preview changes without modifying the scene.
- Use `--index N` if the prefab has multiple components of the same type.

---

## 3. Build Automation with Presets

**Goal:** Build a production player using presets defined in `.udit.yaml`.

First, define a preset in `.udit.yaml`:

```yaml
build:
  targets:
    production:
      target: StandaloneWindows64
      output: Builds/Production
      scenes:
        - Assets/Scenes/Main.unity
        - Assets/Scenes/Level1.unity
      il2cpp: true
```

Then build:

```bash
# 1. Run preflight checks
udit project preflight
# Exits non-zero if there are blocking issues

# 2. Build using the preset
udit build player --config production

# 3. Verify build result
echo "Build output: Builds/Production/"
ls -lh Builds/Production/
```

**Variations:**
- Add `--development` to override IL2CPP with Mono for faster iteration builds.
- Use `udit build cancel` to abort a long-running build.

---

## 4. Asset Cleanup — Find Unreferenced Assets

**Goal:** Identify textures that nothing in the project references.

```bash
# 1. Find all textures in a folder
ASSETS=$(udit asset find --type Texture2D --folder Assets/Art --json \
  | jq -r '.data.matches[].path')

echo "Scanning $(echo "$ASSETS" | wc -l) textures for references..."

# 2. Check references for each
for ASSET in $ASSETS; do
  REFS=$(udit asset references "$ASSET" --limit 1 --json \
    | jq '.data.total')
  if [ "$REFS" -eq 0 ]; then
    echo "  UNREFERENCED: $ASSET"
  fi
done
```

**Tips:**
- This performs a full project scan per asset. For large projects, batch the work or increase `--timeout`.
- To delete unreferenced assets: `udit asset delete "$ASSET"` (moves to trash) or `--permanent` to skip trash.

---

## 5. Log Monitoring

**Goal:** Stream Unity console errors in real time during a play session.

```bash
# Stream errors and exceptions, with user stack frames only
udit log tail --type error --filter "NullReference|MissingComponent" --json
```

Each line is an NDJSON object. Parse with `jq` for structured alerting:

```bash
# Example: count errors per second
udit log tail --type error --json \
  | jq --unbuffered -r '.message' \
  | while read -r MSG; do
      echo "[$(date +%H:%M:%S)] $MSG"
    done
```

**Variations:**
- `--since 5m` to backfill the last 5 minutes before going live.
- `--type error,warning` to include warnings.
- `--stacktrace full` for complete Unity stack traces.

---

## 6. Project Health Report

**Goal:** Generate a quick project status summary (useful for daily standup or dashboards).

```bash
echo "=== Project Info ==="
udit project info --json | jq '{
  unity: .data.unity_version,
  project: .data.project_name,
  scenes: .data.scenes_in_build,
  packages: (.data.packages | length)
}'

echo ""
echo "=== Validation ==="
udit project validate --json | jq '{
  ok: .data.ok,
  errors: (.data.issues | map(select(.severity == "error")) | length),
  warnings: (.data.issues | map(select(.severity == "warning")) | length)
}'

echo ""
echo "=== Console Errors ==="
udit console --type error --json | jq '.data.count'
```

**Tips:**
- Use `udit project preflight` instead of `validate` for a more thorough check (includes build settings and compile state).
- Wrap this in a cron job or a `watch --path` hook for continuous monitoring.

---

## 7. Scene Migration with `.udit.yaml run`

**Goal:** Define a reusable migration task that opens scenes, runs transforms, and saves.

Add a task to `.udit.yaml`:

```yaml
run:
  tasks:
    migrate-lighting:
      steps:
        - udit scene open Assets/Scenes/Main.unity --force
        - udit exec "RenderSettings.ambientMode = UnityEngine.Rendering.AmbientMode.Trilight; return RenderSettings.ambientMode.ToString();"
        - udit scene save
        - udit scene open Assets/Scenes/Level1.unity --force
        - udit exec "RenderSettings.ambientMode = UnityEngine.Rendering.AmbientMode.Trilight; return RenderSettings.ambientMode.ToString();"
        - udit scene save
```

Then run:

```bash
# Preview the steps without executing
udit run migrate-lighting --dry-run

# Execute the migration
udit run migrate-lighting
```

**Tips:**
- Use `udit tx begin` / `udit tx commit` inside steps to group changes into a single Undo entry.
- Add `continue_on_error: true` to the task if some scenes might fail validation.
- Nest tasks: a step can call `udit run <other-task>` (cycle detection is built in).
