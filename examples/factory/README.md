# Factory workload baselines

These are example recipes for the environment contract being developed. The
platform does not yet load or execute `environment.json`; they are not approved
runtime templates. Image admission, service routing, and artifact publication
must be implemented before these recipes can be used as managed environments.

## Research

Requires Python 3.12 or later; no third-party packages, credentials, or network.
Run from `examples/factory/research`:

```sh
python3 experiment.py --output outputs/baseline.json
python3 experiment.py --regularisation 10 --output outputs/variation.json
python3 -m unittest discover -v
```

The fixture records dataset identity, parameters, model coefficient, held-out
error and elapsed time. Identical seed/parameters reproduce the measurements;
execution time is observational. Outputs belong to the research project and are
not committed. This fixture is a baseline for later managed experiment execution.

## GoGallery

The recipe pins source commit `5f1bfd6a41afec65bc7ab791a14863180d944ee0`.
Build/test commands run in a separate checkout of that revision. The Go module
includes Fyne/OpenGL dependencies even for the default binary, so the environment
needs the native compiler, OpenGL and window-system development libraries. Run
these checks inside the approved development sandbox after dependency admission:

```sh
go build -o outputs/gogallery .
go test ./...
```

Generate a fresh synthetic gallery directory, then run the built binary from
that directory with `--config gallery.yaml build`:

```sh
python3 examples/factory/gogallery/create_fixture.py /tmp/gallery-fixture
```

The generator refuses an existing directory and uses absolute input/output paths.
At the pinned revision, relative photo paths caused nested-album lookup failures
in the baseline. The fixture contains three images across nested albums and does
not include personal photos.

The gallery service declaration is a proposed endpoint. Port configuration,
fixture scanning, generated-site serving, and headless GUI requirements still
need a validated recipe. No preview is started by saving the recipe.
