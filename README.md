# promcheck ✔️

<img
  src=".img/logo.png"
  width="150px"
  align="right"
/>

**A tool to identify faulty [Prometheus](https://prometheus.io/) rules**

[![Go Report Card](https://goreportcard.com/badge/github.com/cbrgm/promcheck)](https://goreportcard.com/report/github.com/cbrgm/promcheck)
[![release](https://img.shields.io/github/release-pre/cbrgm/promcheck.svg)](https://github.com/cbrgm/promcheck/releases)
[![license](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/cbrgm/promcheck/blob/master/LICENSE)
![GitHub stars](https://img.shields.io/github/stars/cbrgm/promcheck.svg?label=github%20stars)

## About

**`promcheck` enables you to identify recording or alerting rules using missing metrics or wrong label matchers** (e.g.
because of exporter changes or human-errors).

`promcheck` validates Prometheus [vector selectors](https://prometheus.io/docs/prometheus/latest/querying/basics/) and checks
if they return a result value or not. As a basis for validation, `promcheck` uses Prometheus rule files, but it can also
query rules directly from a running Prometheus instance. It scans the PromQL expression of
each [recording](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/)
and [alerting](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/) rule, takes the individual
referenced selectors out of it and probes them against a remote Prometheus instance.


<div align="center">
<br>
<img src="https://github.com/cbrgm/promcheck/blob/main/.img/demo.gif" width="85%">
<br>
</div>

## Overview
* [About](#about)
* [Installation](#installation)
* [Basic Usage](#basic-usage)
    + [Use-Cases](#use-cases)
    + [Validate rules from a running Prometheus instance](#validate-rules-from-a-running-prometheus-instance)
    + [Validate rules from existing rule files](#validate-rules-from-existing-rule-files)
    + [Validate rules from inline PromQL queries](#validate-rules-from-inline-promql-queries)
    + [Prometheus Exporter](#prometheus-exporter)
* [Configuration](#configuration)
    + [Usage Information](#usage-information)
    + [CI/CD Usage](#cicd-usage)
    + [Output Formats](#output-formats)
* [Container Usage](#container-usage)
* [Kubernetes Deployment](#kubernetes-deployment)
* [Metrics](#metrics)
* [Examples](#examples)
    - [Validating multiple rule groups](#basic-example-validating-multiple-rule-groups)
* [Contributing & License](#contributing---license)

## Installation

`promcheck` is available on Linux, OSX and Windows platforms. Binaries for Linux, Windows and Mac are available as
tarballs in the [release](https://github.com/cbrgm/promcheck/releases) page.

You may also build `promcheck` from source (using Go 1.17+). In order to build `promcheck` from source you must:

* Clone this repository
* Run `make build`

## Basic Usage

`promcheck` can be used in three different modes:

* Validate rules passed to `promcheck` as parameters (`--check.query`)
* Validate rules from existing rule files (`--check.files`)
* Validate rules from a running Prometheus instance (and export results in various formats)

`promcheck` can also be executed as a Prometheus exporter to check a set of rules on a regular basis and export results as scrapeable Prometheus metrics via http.

### Use-Cases

What can you do with this? Possible **use-cases** might be:

* 🛠 Run `promcheck` manually as a cli tool to check rules.
* 🤖 Add `promcheck` to your CI/CD automation pipeline to run integration tests on your rules.
* 📃 Run `promcheck` as an exporter, scrape its metrics and alert in case selectors do not return results anymore.

### Validate rules from a running Prometheus instance

```bash
promcheck --prometheus.url="http://0.0.0.0:9090"
```

Argument Reference:

* `--prometheus.url` - The Prometheus instance to probe selectors against

### Validate rules from existing rule files

```bash
promcheck --prometheus.url="http://0.0.0.0:9090" --check.file=rules.yaml
```

Argument Reference:

* `--prometheus.url` - The Prometheus instance to probe selectors against
* `--check.file` - The Prometheus rule file(s) to validate

<details>
  <summary>Rule group files can be passed in various ways. Click to expand!</summary>

```bash
# validate a rules file `rules.yaml`
promcheck --prometheus.url="http://0.0.0.0:9090" \
          --check.file=rules.yaml

# validate all *.yaml files in directory ./config
promcheck --prometheus.url="http://0.0.0.0:9090" \
          --check.file='./config/*.yaml'
```

</details>

### Validate rules from inline PromQL queries

```bash
promcheck --prometheus.url="http://0.0.0.0:9090" --check.query='up{job="alertmanager-main",namespace="monitoring"}'
```

Argument Reference:

* `--prometheus.url` - The Prometheus instance to probe selectors against
* `--check.query` - Inline PromQL expression (can be passed multiple times)

### Prometheus Exporter

```bash
# example: run promcheck as a prometheus exporter.
# promcheck will validate all rules from the remote instance.
promcheck --prometheus.url="http://0.0.0.0:9090" \
          --exporter.enabled=true

# example: bind on port 9093, run promcheck every 5 min (300 sec.)
promcheck --prometheus.url="http://0.0.0.0:9090" \
          --exporter.enabled=true \
          --exporter.interval=300 \
          --exporter.addr=0.0.0.0:9093

# example: run promcheck as a prometheus exporter.
# promcheck will validate all rules from the rules.yaml file
promcheck --prometheus.url="http://0.0.0.0:9090" \
          --exporter.enabled=true \
          --exporter.interval=300 \
          --exporter.addr=0.0.0.0:9093 \
          --check.file=rules.yaml
```

Argument Reference:

* `--prometheus.url` - The Prometheus instance to probe selectors against
* `--check.file` - The Prometheus rule file(s) to validate.
* `--exporter.enabled` - Run `promcheck` as a Prometheus exporter
* `--exporter.addr` - The exporter's http address
* `--exporter.interval` - The interval in minutes to run `promcheck` and update metrics

## Configuration

For a full list of flags, please also use `promcheck --help`.

```bash
Flags:
  -h, --help                                               Show context-sensitive help.
      --prometheus.url="http://0.0.0.0:9090"               The Prometheus base url
      --prometheus.basic-auth-user=""                      Basic auth username
      --prometheus.basic-auth-pass=""                      Basic auth password
      --check.ignore-selector=CHECK.IGNORE-SELECTOR,...    Regexp of selectors to ignore
      --check.ignore-group=CHECK.IGNORE-GROUP,...          Regexp of rule groups to ignore
      --check.delay=0.1                                    Delay in seconds between probe requests
      --check.file=STRING                                  The rule files to check.
      --check.query=CHECK.QUERY,...                        Inline PromQL expression to check
      --output.format="graph"                              The output format to use
      --output.no-color                                    Toggle colored output
      --exporter.enabled                                   Run promcheck as a prometheus exporter
      --exporter.addr="0.0.0.0:9093"                       The address the http server is running at
      --exporter.interval=300                              Delay in seconds between promcheck runs
      --metrics.profile                                    Enable pprof profiling
      --metrics.runtime                                    Enable runtime metrics
      --metrics.prefix=""                                  Set metrics prefix path
      --log.json                                           Tell promcheck to log json and not key value pairs
      --log.level="info"                                   The log level to use for filtering logs
      --strict                                             Tell promcheck to exit with an error code on expressions without results
```

`promcheck` uses 256 colors terminal mode. On 'nix OS system make sure the `TERM` environment variable is set.

```bash
export TERM=xterm-256color
```

### Usage Information

Keep in mind that `promcheck` may also contain **false positives**, since there may be vector selectors in rules that
intentionally do not return a result value.

`promcheck` does a single HTTP request per vector selector to be probed against the remote Prometheus instance. With many rules to validate, execution time can take longer and lead to many HTTP requests. The interval between probes can be changed with the `--check.delay` flag, which results in fewer requests but increases the runtime of the tool.

### CI/CD Usage

`promcheck` has a flag `--strict`, which causes `promcheck` to terminate with error code `1` after a successful run if expressions without a result value were found.

Therefore, `--strict` should be used, depending on the use case whether `promcheck` should fail the report step during a CI/CD workflow in case of expressions without a result, or whether the step should run successfully regardless of whether expressions have results or not.

### Output formats

Right now, the following output formats are supported:

* `--output.format=graph` - Text format, colored or non-colored (`--output.no-color`) (Default)
* `--output.format=json` - JSON format
* `--output.format=yaml` - YAML format

There might be more formats in near future. Feel free to contribute!

## Container Usage

`promcheck` can also be executed from within a container. The latest container image of `promcheck` is hosted
on [ghcr.io](https://github.com/cbrgm/promcheck).

To run `promcheck` from within a container (assuming that there is a rule file named `rules.yaml` in the current directory), run:

```bash
docker run -v $(pwd):/tmp --rm ghcr.io/cbrgm/promcheck:latest --prometheus.url='http://0.0.0.0:9090' --check.file="/tmp/rules.yaml"
```

To run `promcheck` from within a container as a Prometheus exporter, run:

```bash
docker run --rm -p 9093:9093 ghcr.io/cbrgm/promcheck:latest --prometheus.url='http://0.0.0.0:9090' --exporter.enabled
```

## Kubernetes Deployment

`promcheck` can be executed as a Prometheus exporter to validate a set of rules on a regular basis. Please refer to the [kubernetes.yaml](https://github.com/cbrgm/promcheck/blob/main/kubernetes.yaml) file for a basic deployment example.

## Metrics

* `promcheck_validation_rule_groups_total` - (Gauge) Total number of evaluated rule groups.
* `promcheck_validation_rules_total` - (Gauge) Total number of evaluated rules.
* `promcheck_validation_selectors_total` - (Gauge) Total number of evaluated selectors. Label selectors:
  * `file` - The rules file
  * `group` - The rule group name
  * `rule` - The rule name
  * `status` - The status `failed` or `success`

**Here are some basic examples**:

<details>
  <summary><b>Example: PromQL queries</b> Click to expand!</summary>

Total amount of selectors without result:
```
sum(promcheck_validation_selectors_total{status="failed"})
```

Total amount of selectors without result of rule `KubePodCrashLooping`:

```
promcheck_validation_selectors_total{rule="KubePodCrashLooping", status="failed"}
```

</details>

<details>
  <summary><b>Example: Alert on selectors without a result</b> Click to expand!</summary>

```yaml
groups:
  - name: example
    rules:
      # alert definition
      - alert: HighRequestLatency
        expr: job:request_latency_seconds:mean5m{job="myjob"} > 0.5
        for: 10m
        labels:
          severity: page
        annotations:
          summary: High request latency
      # alert in case HighRequestLatency selectors are not returning results
      - alert: HighRequestLatencyMissingMetrics
          expr: promcheck_validation_selectors_total{rule="HighRequestLatency", status="failed"} > 0
          for: 1m
          labels:
            severity: warning
          annotations:
            summary: HighRequestLatency uses selectors without result values.
```

</details>

## Examples

Please refer below for some basic usage examples demonstrating what `promcheck` can do for you!

#### Basic Example validating multiple rule groups

**Input:**
<details>
  <summary>rules.yaml (Click to expand!)</summary>

```yaml
"groups":
  - "name": "kubernetes-apps-demo-group"
    "rules":
      - "alert": "KubePodCrashLooping"
        "annotations":
          "description": "Pod {{ $labels.namespace }}/{{ $labels.pod }} ({{ $labels.container }}) is in waiting state (reason: \"CrashLoopBackOff\")."
          "runbook_url": "https://github.com/kubernetes-monitoring/kubernetes-mixin/tree/master/runbook.md#alert-name-kubepodcrashlooping"
          "summary": "Pod is crash looping."
        "expr": |
          max_over_time(kube_pod_container_status_waiting_reason{reason="CrashLoopBackOff", job="kube-state-metrics"}[5m]) >= 1
        "for": "15m"
        "labels":
          "severity": "warning"
      - "alert": "KubePodNotReady"
        "annotations":
          "description": "Pod {{ $labels.namespace }}/{{ $labels.pod }} has been in a non-ready state for longer than 15 minutes."
          "runbook_url": "https://github.com/kubernetes-monitoring/kubernetes-mixin/tree/master/runbook.md#alert-name-kubepodnotready"
          "summary": "Pod has been in a non-ready state for more than 15 minutes."
        "expr": |
          sum by (namespace, pod) (
            max by(namespace, pod) (
              kube_pod_status_phase{job="kube-state-metrics", phase=~"Pending|Unknown"}
            ) * on(namespace, pod) group_left(owner_kind) topk by(namespace, pod) (
              1, max by(namespace, pod, owner_kind) (kube_pod_owner{owner_kind!="Job"})
            )
          ) > 0
        "for": "15m"
        "labels":
          "severity": "warning"
  - "name": "kubernetes-system-scheduler-demo-group"
    "rules":
      - "alert": "KubeSchedulerDown"
        "annotations":
          "description": "KubeScheduler has disappeared from Prometheus target discovery."
          "runbook_url": "https://github.com/kubernetes-monitoring/kubernetes-mixin/tree/master/runbook.md#alert-name-kubeschedulerdown"
          "summary": "Target disappeared from Prometheus target discovery."
        "expr": |
          absent(up{job="kube-scheduler"} == 1)
        "for": "15m"
        "labels":
          "severity": "critical"
  - "name": "kubernetes-system-controller-manager-demo-group"
    "rules":
      - "alert": "KubeControllerManagerDown"
        "annotations":
          "description": "KubeControllerManager has disappeared from Prometheus target discovery."
          "runbook_url": "https://github.com/kubernetes-monitoring/kubernetes-mixin/tree/master/runbook.md#alert-name-kubecontrollermanagerdown"
          "summary": "Target disappeared from Prometheus target discovery."
        "expr": |
          absent(up{job="kube-controller-manager"} == 1)
        "for": "15m"
        "labels":
          "severity": "critical"

```

</details>

**Command**:

```bash
➜ ./promcheck --check.file 'rules.yaml' --prometheus.url http://0.0.0.0:9090
```

* Prometheus instance running locally on `http://0.0.0.0:9090`

**Output**:

```
.
└── [file] examples/rules_multiple_groups.yaml
    ├── [group] kubernetes-apps-demo-group
    │   ├── [0/1] KubePodCrashLooping
    │   │   └── [✖] kube_pod_container_status_waiting_reason{job="kube-state-metrics",reason="CrashLoopBackOff"}
    │   └── [2/2] KubePodNotReady
    │       ├── [✔] kube_pod_status_phase{job="kube-state-metrics",phase=~"Pending|Unknown"}
    │       └── [✔] kube_pod_owner{owner_kind!="Job"}
    ├── [group] kubernetes-system-scheduler-demo-group
    │   └── [1/1] KubeSchedulerDown
    │       └── [✔] up{job="kube-scheduler"}
    └── [group] kubernetes-system-controller-manager-demo-group
        └── [1/1] KubeControllerManagerDown
            └── [✔] up{job="kube-controller-manager"}

Rules validated total: 4
Selectors total: 5, Results found: 4, No Results found 1 (Failed/Total: 20.00%)
```

**Command**:

Ignore rule group `kubernetes-system-controller-manager-demo-group`

```bash
➜ ./promcheck --check.file 'rules.yaml' \
              --check.ignore-group 'kubernetes-system-controller-manager-demo-group' \
              --prometheus.url http://0.0.0.0:9090
```

* Prometheus instance running locally on `http://0.0.0.0:9090`

**Output**:

```
.
└── [file] examples/rules_multiple_groups.yaml
    ├── [group] kubernetes-apps-demo-group
    │   ├── [2/2] KubePodNotReady
    │   │   ├── [✔] kube_pod_status_phase{job="kube-state-metrics",phase=~"Pending|Unknown"}
    │   │   └── [✔] kube_pod_owner{owner_kind!="Job"}
    │   └── [0/1] KubePodCrashLooping
    │       └── [✖] kube_pod_container_status_waiting_reason{job="kube-state-metrics",reason="CrashLoopBackOff"}
    └── [group] kubernetes-system-scheduler-demo-group
        └── [1/1] KubeSchedulerDown
            └── [✔] up{job="kube-scheduler"}

Rules validated total: 4
Selectors total: 4, Results found: 3, No Results found 1 (Failed/Total: 25.00%)
```

**Command**:

Output json:

```bash
➜ ./promcheck --check.file 'rules.yaml' \
              --check.ignore-group 'kubernetes-system-controller-manager-demo-group' \
              --prometheus.url http://0.0.0.0:9090
              --output.format json
```

**Output**:

```json
{
  "promcheck": {
    "results": [
      {
        "file": "examples/rules_multiple_groups.yaml",
        "group": "kubernetes-apps-demo-group",
        "name": "KubePodCrashLooping",
        "expression": "max_over_time(kube_pod_container_status_waiting_reason{reason=\"CrashLoopBackOff\", job=\"kube-state-metrics\"}[5m]) \u003e= 1\n",
        "no_results": [
          "kube_pod_container_status_waiting_reason{job=\"kube-state-metrics\",reason=\"CrashLoopBackOff\"}"
        ],
        "results": []
      },
      {
        "file": "examples/rules_multiple_groups.yaml",
        "group": "kubernetes-apps-demo-group",
        "name": "KubePodNotReady",
        "expression": "sum by (namespace, pod) (\n  max by(namespace, pod) (\n    kube_pod_status_phase{job=\"kube-state-metrics\", phase=~\"Pending|Unknown\"}\n  ) * on(namespace, pod) group_left(owner_kind) topk by(namespace, pod) (\n    1, max by(namespace, pod, owner_kind) (kube_pod_owner{owner_kind!=\"Job\"})\n  )\n) \u003e 0\n",
        "no_results": [],
        "results": [
          "kube_pod_status_phase{job=\"kube-state-metrics\",phase=~\"Pending|Unknown\"}",
          "kube_pod_owner{owner_kind!=\"Job\"}"
        ]
      },
      {
        "file": "examples/rules_multiple_groups.yaml",
        "group": "kubernetes-system-scheduler-demo-group",
        "name": "KubeSchedulerDown",
        "expression": "absent(up{job=\"kube-scheduler\"} == 1)\n",
        "no_results": [],
        "results": [
          "up{job=\"kube-scheduler\"}"
        ]
      }
    ],
    "groups_total": 2,
    "rules_warnings": 3,
    "rules_total": 4,
    "selectors_success_total": 3,
    "selectors_failed_total": 1,
    "ratio_failed_total": 25.00
  }
}
```

## Contributing & License

We welcome and value your contributions to this project! 👍 If you're interested in making improvements or adding features, please refer to our [Contributing Guide](https://github.com/cbrgm/promcheck/blob/main/CONTRIBUTING.md). This guide provides comprehensive instructions on how to submit changes, set up your development environment, and more.

Please note that this project is developed in my spare time and is available for free 🕒💻. As an open-source initiative, it is governed by the [Apache 2.0 License](https://github.com/cbrgm/promcheck/blob/main/LICENSE). This license outlines your rights and obligations when using, modifying, and distributing this software.

Your involvement, whether it's through code contributions, suggestions, or feedback, is crucial for the ongoing improvement and success of this project. Together, we can ensure it remains a useful and well-maintained resource for everyone 🌍.
