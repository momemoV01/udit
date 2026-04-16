# Writing Custom Tools

> See the [README](../README.md) for installation and quick start. This page covers extending udit with project-specific tools.

Create a static class with `[UditTool]` attribute in any Editor assembly. The Connector discovers it automatically on domain reload.

```csharp
using UditConnector;
using Newtonsoft.Json.Linq;
using UnityEngine;

[UditTool(Name = "spawn", Description = "Spawn an enemy at a position", Group = "gameplay")]
public static class SpawnEnemy
{
    public class Parameters
    {
        [ToolParameter("X world position", Required = true)]
        public float X { get; set; }

        [ToolParameter("Y world position", Required = true)]
        public float Y { get; set; }

        [ToolParameter("Z world position", Required = true)]
        public float Z { get; set; }

        [ToolParameter("Prefab name in Resources folder", DefaultValue = "Enemy")]
        public string Prefab { get; set; }
    }

    public static object HandleCommand(JObject parameters)
    {
        var p = new ToolParams(parameters);
        float x = p.GetFloat("x", 0);
        float y = p.GetFloat("y", 0);
        float z = p.GetFloat("z", 0);
        string prefabName = p.Get("prefab", "Enemy");

        var prefab = Resources.Load<GameObject>(prefabName);
        var instance = Object.Instantiate(prefab, new Vector3(x, y, z), Quaternion.identity);

        return new SuccessResponse("Enemy spawned", new
        {
            name = instance.name,
            position = new { x, y, z }
        });
    }
}
```

Call it directly with flags or JSON:

```bash
udit spawn --x 1 --y 0 --z 5 --prefab Goblin
udit spawn --params '{"x":1,"y":0,"z":5,"prefab":"Goblin"}'
```

**Key points:**

- **Name**: without `Name`, auto-derived from class name (`SpawnEnemy` → `spawn_enemy`, `UITree` → `ui_tree`). With `Name = "spawn"`, the command becomes `udit spawn`.
- **Parameters class**: optional but recommended. `udit list` uses it to expose parameter names, types, descriptions, and required flags — so AI assistants can discover your tool without reading the source.
- **ToolParams**: use `p.Get()`, `p.GetInt()`, `p.GetFloat()`, `p.GetBool()`, `p.GetRaw()` for consistent param reading.
- **Discovery**: `udit list` shows built-in tools first (`group: "built-in"`), then custom tools (`group: "custom"`) detected from the connected Unity project.

**Attribute reference:**

| Attribute | Property | Description |
|---|---|---|
| `[UditTool]` | `Name` | Command name override (default: class name → snake_case) |
| | `Description` | Tool description shown in `list` |
| | `Group` | Group name for categorization |
| `[ToolParameter]` | `Description` | Parameter description (constructor arg) |
| | `Required` | Whether the parameter is required (default: `false`) |
| | `Name` | Parameter name override |
| | `DefaultValue` | Default value hint |

### Rules

- Class must be `static`
- Must have `public static object HandleCommand(JObject parameters)` or `async Task<object>` variant
- Return `SuccessResponse(message, data)` or `ErrorResponse(message)`
- Add a `Parameters` nested class with `[ToolParameter]` attributes for discoverability
- Class name is auto-converted to snake_case for the command name
- Override with `[UditTool(Name = "my_name")]` if needed
- Runs on Unity main thread, so all Unity APIs are safe to call
- Discovered automatically on Editor start and after every script recompilation
- Duplicate tool names are detected and logged as errors — only the first discovered handler is used
