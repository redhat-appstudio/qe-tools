# Estimating time to review a PR

## Usage
See the [GitHub Action workflow file](../.github/workflows/estimate-review.yml)

Everything should be already setup, but you might want to change the `CONFIG_PATH` environment variable.

For setting up more specific workflows, run `./qe-tools estimate-review --help` to see the usage

```
Estimate time needed to review a PR in seconds

Usage:
  qe-tools estimate-review [flags]

Flags:
      --add-label           add label to the GitHub PR
      --config string       path to the yaml config file
  -h, --help                help for estimate-review
      --human               human readable form
      --number int          number of the pull request (default 1)
      --owner string        owner of the repository (default "konflux-ci")
      --repository string   name of the repository (default "e2e-tests")
      --token string        GitHub token
```

## How it works

The tool takes following parameters into account:
- additions
- deletions
- extensions of changed files
- number of changed files
- number of commits

It sums additions and deletions considering their weights and the weight of the extension of the changed file.

`(additions*BASE_COEF + deletions*DELETE_COEF) * EXT_COEF` 

Then it multiplies this equation with coefficients of 
- commits `(1 + COMMIT_COEF * (commits-1))` 
- number of changed files `(1 + FILE_COEF * (files-1))`

These coefficients support having a ceiling (won't go above the specified value).

## Configuration
Configuration is done via a yaml file. 

**Please take a look at the provided [YAML config file](../config/estimate/config.yaml)**

### Required
#### labels
Having at least one label in the config is required if the `add-label` flag is present. 

A label should have following parameters:
- `name`: Label text that you want on the pull request  (string)
- `time`: the lower bound when the label can be applied (in seconds, int)

e.g. Label that should be applied if the review time takes longer than 60 second:
```yaml
labels:
  - name: "> 1 min"
    time: 60
```

### Recommended
#### extensions
Weights that should be applied on changes in a particular file extension.

`default` is a special extension which specifies a weight for extensions not found in the list.

e.g. Weights for default, py and md extensions where:
- .py files take 1 unit of time to review
- .md files take 0.2 units of time
- all the other files take 2 units of time
```yaml
extensions:
  - default: 2
  - py: 1
  - md: 0.2
```

### Optional
- `base`: base weight, used for additions (default 1)
- `deletion`: weight used for deletions (default 0.5)
- `commit`: weight and ceiling used for calculating the commit coefficient
  - `weight`: weight of one commit (default 0.05)
  - `ceiling`: maximum value of the coefficient (default 2)
- `files`: same idea as commit coefficient but for number of changed files
  - `weight`: (default 0.1)
  - `ceiling`: (default 2)
