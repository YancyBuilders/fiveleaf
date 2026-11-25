# Python Debugger

Debug Python with [debugpy](https://github.com/microsoft/debugpy).

Install debugpy in the Python environment that runs the program, then enable the plugin in **Settings → Plugins**:

```sh
python3 -m pip install debugpy
```

If `python3` is not the interpreter of the project, change the adapter's `command` to its absolute path.

Set a breakpoint in the editor margin or press `F9`, then press `F5`.
