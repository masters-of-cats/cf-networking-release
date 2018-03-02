# 3rd Party Plugin Development for Container Networking

## Table of Contents

- Introduction
- Architecture
- Mandatory Features
   - NetOut
   - NetIn
   - Policy Configuration
   - MTU
   - Bosh DNS
   - Plugin is a bosh release
- Optional Capabilities
   - Per ASG Logging
   - Global ASG and Container-to-Continer Logging
   - Bosh Backup and Restore
- Getting Data from CF
   - From the Config
   - From the Internal Policy Server
   - From CAPI
   - From  BBS
- Common Gotchas

## Introduction

So you want to create your own CNI plugin with Cloud Foundry? 

First, all CNI plugins are required to implement [this set of features](https://github.com/containernetworking/cni/blob/master/SPEC.md). 

Cloud Foundry requires the networking stack to perform certain additional functions which are currently not standardized by CNI. These are: These are spelled out later in this doc in more detail [here](TODO: link to mandatory features). 

There are also associated [tests suites](TODO: link to tests section) to confirm the plugin implementation is correct. 

## Architecture

*If you want to integrate your own CNI plugin with Cloud Foundry, begin by reviewing the component diagrams on the [architecture page](arch.md). Note that your plugin would replace the components in red, and take on the responsibilities of these components.*

## Mandatory features

In addition to the features listed in the [CNI spec](), the following features are required.

- NetOut
- NetIn
- Policy configuration
- MTU
- Bosh DNS
- Your CNI plugin is a bosh release

### NetOut
**Spec**: Operators can configure ASGs at the CF or space level to allow traffic from apps and tasks to CIDR ranges.

**Description**: Networking layer provides IP addressing and connectivity for containers. The networking layer sets up firewall rules to allow traffic based on ASG configuration. For more information on ASGs, see [these docs](https://docs.cloudfoundry.org/concepts/asg.html).

**CF Information Needed**: ASG information can be pulled from the config passed in from the garden external networker. See [`runtimeConfig.netOutRules`](). The ASG information provided there will only be for the ASGs that are currently applied to the app. If you want information about new ASGs has been added through Cloud Controller, but that haven't been passed through on the config because the app has not been restarted, you can [poll CAPI]().

### NetIn
**Spec**: External entities can reach applications through the GoRouter.

**Description**: Networking layer sets up firewall rules to allow ingress traffic from GoRouter, TCP router and SSH proxy. 

**CF Information Needed**: In order for the GoRouter, TCP router, and SSH proxy to be able to access your app, ports listed in `portMappings` need to be exposed via DNAT. For example, the cni-wrapper-plugin insilk-release[link to config passed in to cni-wrapper-plugin further in the doc here] - See `runtimeConfig.portMappings`. These can also be retreived from [environment variables](https://docs.run.pivotal.io/devguide/deploy-apps/environment-variable.html#CF-INSTANCE-PORTS)

### Policy Confirguration
**Spec**: App-to-app policies between app containers and task containers for those apps

**Description**: The networking layer sets up firewall rules to allow container-to-container traffic based on policy  (v1 of policy API must be supported). 

**CF Information Needed**: You need to have an agent running that is polling the internal policy server. [Link to how to poll the internal policy server]. For exmple, VXLAN Policy Agent. 

### MTU

**Spec**: operatros can override the MTU on the interface

**Description**: CNI plugins should automatically detect the MTU settings on the host, and set the MTU
on container network interfaces appropriately.  For example, if the host MTU is 1500 bytes
and the plugin encapsulates with 50 bytes of header, the plugin should ensure that the
container MTU is no greater than 1450 bytes.  This is to ensure there is no fragmentation.
The built-in silk CNI plugin does this.

Operators may wish to override the MTU setting. In this case they will set the BOSH property [cf_networking.mtu](http://bosh.io/jobs/cni?source=github.com/cloudfoundry/cf-networking-release#p=cf_networking.mtu).
3rd party plugins should respect this value. 

**CF Information Needed**: This value will be included in the config object that is passed to the configured [CNI plugin](TODO).

### Bosh DNS
**Spec**: Apps can connect to services using [Bosh DNS](TODO). 

**Description**: The networking layer allows containers to reach Bosh DNS on the cell at `169.254.0.2`. The CNI plugin returns the bosh DNS address as `169.254.0.2`.

### Your CNI plugin is a bosh release

#### To author a BOSH release with your plugin

Your CNI plugin will need to be packaged as a [BOSH release](http://bosh.io/docs#release). 

Add in all packages and jobs required by your CNI plugin.  At a minimum, you must provide a CNI binary program and a CNI config file.
   If your software requires a long-lived daemon to run on the diego cell, we recommend you deploy a separate BOSH job for that.
  - For more info on **bosh packaging scripts** read [this](http://bosh.io/docs/packages.html#create-a-packaging-script).
  - For more info on **bosh jobs** read [this](http://bosh.io/docs/jobs.html).

Use the [silk-release](http://github.com/cloudfoundry/silk-release) as inspiration.

#### To deploy your BOSH release with Cloud Foundry

Update the [deployment manifest properties](http://bosh.io/docs/deployment-manifest.html#properties)
    - The garden cni job properties must be configured to point to your plugin's paths.  

  ```yaml
  properties:
    cf_networking:
      cni_plugin_dir: /var/vcap/packages/YOUR_PACKAGE/bin # directory for CNI binaries
      cni_config_dir: /var/vcap/jobs/YOUR_JOB/config/cni  # directory for CNI config file(s)
  ```
The above properties are configured on the garden-cni job: [`cni_config_dir`](http://bosh.io/jobs/garden-cni?source=github.com/cloudfoundry/cf-networking-release#p=cf_networking.cni_config_dir) and [`cni_plugin_dir`](http://bosh.io/jobs/garden-cni?source=github.com/cloudfoundry/cf-networking-release#p=cf_networking.cni_plugin_dir)

-- TODO: Is this yaml too mysterious?

## Optional capabilities
The following features are optional for your CNI plugin:
- Per ASG logging
- Global ASG and container-to-container logging
- Bosh backup and restore (BBR)

### Per ASG Logging
**Spec**: Operaters can configure `log: true` in ASG config.

**Description**: The networking layer logs all accepted/denied packets for the ASG with `log: true` set.

**CF Information Needed**: ASG information can be pulled from the config passed in from the garden external networker. See [`runtimeConfig.netOutRules`](). 

### Global ASG and Container-to-Container Logging
**Spec**: Operators can enable global logging in PAS tile

**Description**: The networking later logs all accepted/denied ASG and container-to-container packets. 

**CF Information Needed**: ASG information can be pulled from the config passed in from the garden external networker. See [`runtimeConfig.netOutRules`](). TODO: where can this flag be found? 

### Bosh Backup and Restore (BBR)
**Spec**: Operators can backup and restore Bosh deployments

**Description**: Add support for BBR scripts if there is data that must be retained after a backup and restore operation. 

**CF Information Needed**: ?? TODO

## Getting Data from CF
### From Config

This config is described in the [CNI conventions document](https://github.com/containernetworking/cni/blob/master/CONVENTIONS.md).

The `garden-external-networker` will invoke one or more CNI plugins, according to the [CNI Spec](https://github.com/containernetworking/cni/blob/master/SPEC.md).
It will start with the CNI config files available in the [`cni_config_dir`](http://bosh.io/jobs/garden-cni?source=github.com/cloudfoundry/cf-networking-release#p=cf_networking.cni_config_dir) and also inject
some dynamic information about the container. This is divided into two keys the first, `metadata`
contains the CloudFoundry App, Space and Org that it belongs to. Another key `runtimeConfig` holds information that CNI plugins may need
to implement legacy networking features of Cloud Foundry. It is divided into two keys, `portMappings` can be translated into port forwarding
rules to allow the gorouter access to application containers, and `netOutRules` which are egress whitelist rules used for implementing
application security groups.

A reference implementation of these features can be seen in the [cni-wrapper-plugin](../src/cni-wrapper-plugin).

For example, at deploy time, Silk's CNI config is generated from this [template](../jobs/silk-cni/templates/cni-wrapper-plugin.conf.erb), and
is stored in a file on disk at `/var/vcap/jobs/silk-cni/config/cni-wrapper-plugin.conf`, which resembles

```json
{
  "name": "cni-wrapper",
  "type": "cni-wrapper-plugin",
  "cniVersion": "0.3.1",
  "datastore": "/var/vcap/data/container-metadata/store.json",
  "iptables_lock_file": "/var/vcap/data/garden-cni/iptables.lock",
  "overlay_network": "10.255.0.0/16",
  "instance_address": "10.0.16.14",
  "iptables_asg_logging": true,
  "iptables_c2c_logging": true,
  "ingress_tag": "ffff0000",
  "dns_servers": [

  ],
  "delegate": {
    "cniVersion": "0.3.1",
    "name": "silk",
    "type": "silk-cni",
    "daemonPort": 23954,
    "dataDir": "/var/vcap/data/host-local",
    "datastore": "/var/vcap/data/silk/store.json",
    "mtu": 0
  }
}
```

Then, when a container is created, the `garden-external-networker` adds additional runtime-specific data, so that
the CNI plugin receives a final config object that resembles:

```json
{
  "name": "cni-wrapper",
  "type": "cni-wrapper-plugin",
  "cniVersion": "0.3.1",
  "datastore": "/var/vcap/data/container-metadata/store.json",
  "iptables_lock_file": "/var/vcap/data/garden-cni/iptables.lock",
  "overlay_network": "10.255.0.0/16",
  "instance_address": "10.0.16.14",
  "iptables_asg_logging": true,
  "iptables_c2c_logging": true,
  "ingress_tag": "ffff0000",
  "dns_servers": [

  ],
  "delegate": {
    "cniVersion": "0.3.1",
    "name": "silk",
    "type": "silk-cni",
    "daemonPort": 23954,
    "dataDir": "/var/vcap/data/host-local",
    "datastore": "/var/vcap/data/silk/store.json",
    "mtu": 0
  },
  "runtimeConfig": {
    "portMappings": [{
      "host_port": 60001,
      "container_port": 8080
    }, {
      "host_port": 60002,
      "container_port": 2222
    }],
    "netOutRules": [{
      "protocol": 1,
      "networks": [{
        "start": "8.8.8.8",
        "end": "9.9.9.9"
      }],
      "ports": [{
        "start": 53,
        "end": 54
      }],
      "log": true
    }],
    "metadata": {
      "policy_group_id": "d5bbc5ed-886a-44e6-945d-67df1013fa16",
      "app_id": "d5bbc5ed-886a-44e6-945d-67df1013fa16",
      "space_id": "4246c57d-aefc-49cc-afe0-5f734e2656e8",
      "org_id": "2ac41bbf-8eae-4f28-abab-51ca38dea3e4"
    }
  }
}
```

The metadata section includes data that will be required to interact with the policy server. Your plugin will be expected to allow traffic between application containers according to the responses retrieved from the policy server.

Furthermore, the CNI runtime data, provided as environment variables, sets the
[CNI `ContainerID`](https://github.com/containernetworking/cni/blob/master/SPEC.md#parameters) equal to the
[Garden container `Handle`](https://godoc.org/code.cloudfoundry.org/garden#ContainerSpec).

When [Diego](https://github.com/cloudfoundry/diego-release) calls Garden, it sets that equal to the [`ActualLRP` `InstanceGuid`](https://godoc.org/code.cloudfoundry.org/bbs/models#ActualLRPInstanceKey).
In this way, a 3rd-party system can relate data from CNI with data in the [Diego BBS](https://github.com/cloudfoundry/bbs/tree/master/doc).
### Information from Internal Policy Server

3rd party CNI plugins are expected to implement the features necessary to allow application containers to access on another. The policies that are created by CF users are retrieved from the Internal Policy Server. Clients to this api will need to poll this api to ensure the changes to the policies are honored.

3rd party integrators should expect the internal policy server component will be present in a standard CF deploy.

If you are replacing the built-in "VXLAN Policy Agent" with your own Policy Enforcement implementation, you can use the Policy Server's internal API to retrieve policy information.

For how to use the Internal Policy Server API, [read here](API.md).

### Information from CAPI
#### Poll for ASG
### From BBS
#### Subscribe to BBS event stream for receiving LRP events

## Tests

A Cloud Foundry system that integrates a 3rd party networking component should be able to pass the following test suites:

- [CF Networking Smoke Tests](../src/test/smoke)
- [CF Networking Acceptance Tests](../src/test/acceptance)
- [CF Acceptance Tests (CATs)](https://github.com/cloudfoundry/cf-acceptance-tests/)
- [CF Routing Acceptance Tests (RATS)](https://github.com/cloudfoundry-incubator/routing-acceptance-tests)
- Optional - [CF Disaster Recovery Acceptance Tests (DRATS)](https://github.com/cloudfoundry-incubator/disaster-recovery-acceptance-tests)

Only the `CF Networking Smoke Tests` are non-disruptive and may be run against a live, production environment.  The other tests make potentially disruptive changes and should only be run against a non-production environment.

For local development, we recommend using [`cf-deployment` on BOSH-lite](https://github.com/cloudfoundry/cf-deployment).

For guidance on these test suites, please reach out to our team in Slack (top of this page).

## Common Gotchas

COMMON GOTCHA: If you want to integrate using the default values for the [`cni_config_dir`](http://bosh.io/jobs/garden-cni?source=github.com/cloudfoundry/cf-networking-release#p=cf_networking.cni_config_dir) and [`cni_plugin_dir`](http://bosh.io/jobs/garden-cni?source=github.com/cloudfoundry/cf-networking-release#p=cf_networking.cni_plugin_dir), your BOSH package for the CNI plugin *must* be named `cni` and the BOSH job for the CNI plugin *must* be named `cni`.

If you have any questions or feedback, please visit the `#container-networking` channel on [Cloud Foundry Slack](http://slack.cloudfoundry.org/).
