# Network Policy Recommendation

## Table of Contents

<!-- toc -->
- [Introduction](#introduction)
- [Deployment](#deployment)
- [Command-line Usage](#command-line-usage)
  - [Start a policy recommendation job](#start-a-policy-recommendation-job)
  - [Check the status of a policy recommendation job](#check-the-status-of-a-policy-recommendation-job)
  - [Get the result of a policy recommendation job](#get-the-result-of-a-policy-recommendation-job)
<!-- /toc -->

## Introduction

Network policy recommendation is a feature that utilizes the flow records collected by
[Grafana Flow Collector](network-flow-visibility.md#grafana-flow-collector) and generate
Kubernetes or [Antrea network policies](https://github.com/antrea-io/antrea/blob/main/docs/antrea-network-policy.md).
The goal of this feature is to aid the cluster admin or developers to secure
their applications and control network traffic based on the idea of the Zero Trust model.

## Deployment

To use the network policy recommendation feature, please follow this [doc](https://github.com/antrea-io/antrea/blob/main/docs/network-flow-visibility.md) to deploy Flow Exporter and Flow Aggregator first.
Please update the `#podLabels: false` to `podLabels: true` inside
[Flow Aggregator Configuration](https://github.com/antrea-io/antrea/blob/main/docs/network-flow-visibility.md#configuration-1),
because policy recommendation feature needs Pod lables information to recommend network policies.

Then, please follow these [deployment steps](network-flow-visibility.md#deployment-steps)
to deploy the Grafana Flow Collector.

After we finish the deployment of Grafana Flow Collector and can see some flow records in the Grafana UI,
we can continue to deploy other necessary components of policy recommendation feature:

We need to install the [Kubernetes Operator for Apache Spark](https://github.com/GoogleCloudPlatform/spark-on-k8s-operator)
in the cluster.
Our policy recommendation logic is implemented as a [Spark](https://github.com/apache/spark) application,
the Kubernetes Operator for Apache Spark helps us schedule the Spark job in the Kubernetes cluster.

Please run the following command:

```bash
helm repo add spark-operator https://googlecloudplatform.github.io/spark-on-k8s-operator
helm install policy-reco spark-operator/spark-operator --namespace flow-visibility --set image.tag=latest
```

This will install the Kubernetes Operator for Apache Spark into the `flow-visibility` namespace.

Once we finish these deployment steps, we should see `policy-reco-spark-operator` Pods running inside the `flow-visibility` namespace:

```bash
$ kubectl get pods -n flow-aggregator
NAME                                          READY   STATUS    RESTARTS   AGE
policy-reco-spark-operator-56c4cb454c-4vhfh   1/1     Running   0          2m19s
```

## Command-line Usage

Currently, the network policy recommendation feature only supports command line interaction through Theiactl.
Theiactl is the Theia command-line tool. To get more information about Theiactl, please refer to this [doc](./theiactl.md). We have 3 Theiactl commands for the policy recommendation feature:

- `theiactl policyreco start`
- `theiactl policyreco check`
- `theiactl policyreco result`

To see all options and usage examples of these commands, you may run `theiactl policyreco [subcommand] --help`. In the following sections, we will go through a simple example to utilize these commands with default options.

### Start a policy recommendation job

The `theiactl policyreco start` command can start a new policy recommendation job.
If the new policy recommendation job is created successfully, the `recommendation ID` of this job will be returned:

```bash
$ theiactl policyreco start
Policy recommendation start successfully, id is e998433e-accb-4888-9fc8-06563f073e86
```

### Check the status of a policy recommendation job

After we start a policy recommendation job, we could use the `theiactl policyreco check`
command can check the current status of that job.

For the policy recommendation job we just start above, we could check the status of it by:

```bash
$ theiactl policyreco check --id e998433e-accb-4888-9fc8-06563f073e86
Status of this policy recommendation job is COMPLETED
```

It will return the status of this Spark application like `SUBMITTED`, `RUNNING`, `COMPLETED`, and `FAILED`.

For a complete list of the possible status of a spark application, please refer to the definition [here](https://github.com/GoogleCloudPlatform/spark-on-k8s-operator/blob/3b58b2632545b1f20e105a6080d6597513af60da/pkg/apis/sparkoperator.k8s.io/v1beta2/types.go#L331).

### Get the result of a policy recommendation job

After a policy recommendation job completes, the recommended policies will be written into the Clickhouse database. We could use the `theiactl policyreco result` command to get the result:

```bash
$ theiactl policyreco result --id e998433e-accb-4888-9fc8-06563f073e86
apiVersion: crd.antrea.io/v1alpha1
kind: ClusterNetworkPolicy
metadata:
name: recommend-allow-acnp-kube-system-q7loe
spec:
appliedTo:
- namespaceSelector:
    matchLabels:
        kubernetes.io/metadata.name: kube-system
egress:
- action: Allow
    to:
    - podSelector: {}
ingress:
- action: Allow
    from:
    - podSelector: {}
priority: 5
tier: Platform
---
... other policies
```

To apply recommended policies in the cluster, we can save the output of this command to a yml file and apply it through `kubectl`:

```bash
theiactl policyreco result --id e998433e-accb-4888-9fc8-06563f073e86 > recommended_policies.yml
kubectl apply -f recommended_policies.yml
```
