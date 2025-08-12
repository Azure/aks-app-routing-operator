# Change Log

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.8] - 2025-08-11

### Changed
- bumped external dns to 0.17.0 - [link](https://github.com/Azure/aks-app-routing-operator/pull/462)

### Added
- workload identity for ingress keyvault - [link](https://github.com/Azure/aks-app-routing-operator/pull/459)
- default domain certificate crd - [link](https://github.com/Azure/aks-app-routing-operator/pull/468)
- flag to enable default domain - [link](https://github.com/Azure/aks-app-routing-operator/pull/469)
- default domain reconciler to reconcile default domain into cluster - [link](https://github.com/Azure/aks-app-routing-operator/pull/473)
- tls validation for default domain - [link](https://github.com/Azure/aks-app-routing-operator/pull/477)
- store to watch for default domain changes and requeue - [link](https://github.com/Azure/aks-app-routing-operator/pull/479), [2](https://github.com/Azure/aks-app-routing-operator/pull/481)

## [0.2.7] - 2025-06-19

### Changed
- changed ip logging flag to internal logging and switched format to json - [link](https://github.com/Azure/aks-app-routing-operator/pull/449)

## [0.2.6] - 2025-05-30

### Added
- ssl passthrough configuration option - [link](https://github.com/Azure/aks-app-routing-operator/pull/440)
- custom logging configuration option - [link](https://github.com/Azure/aks-app-routing-operator/pull/439)
- load balancer source ip ranges configuration option - [link](https://github.com/Azure/aks-app-routing-operator/pull/437)
- client ip logging flag - [link](https://github.com/Azure/aks-app-routing-operator/pull/436)
- firewall bypass with kubernetes.azure.com/set-kube-service-host-fqdn flag - [link](https://github.com/Azure/aks-app-routing-operator/pull/434)


## [0.2.5] - 2025-04-22

### Added
- log ingresses using managed controllers - [link](https://github.com/Azure/aks-app-routing-operator/pull/206)
- default-backend-service to crd - [link](https://github.com/Azure/aks-app-routing-operator/pull/174)
- structured logging to setup logs - [link](https://github.com/Azure/aks-app-routing-operator/pull/258)
- custom-http-errors field to crd - [link](https://github.com/Azure/aks-app-routing-operator/pull/201)
- aks-managed-by label to top level labels - [link](https://github.com/Azure/aks-app-routing-operator/pull/335)
- Gateway API TLS functionality with Gateway API via workload identity - [link](https://github.com/Azure/aks-app-routing-operator/pull/306)
- CRD for ExternalDNS - [link](https://github.com/Azure/aks-app-routing-operator/pull/320)
- HTTP disabled flag for NginxIngressController CRD - [link](https://github.com/Azure/aks-app-routing-operator/pull/358),[link2](https://github.com/Azure/aks-app-routing-operator/pull/362)
- bump pause image to 3.10 - [link](https://github.com/Azure/aks-app-routing-operator/pull/373)
- respect rollouts in topology spread - [link](https://github.com/Azure/aks-app-routing-operator/pull/374)
- add delay to ingress-nginx controller shutdown - [link](https://github.com/Azure/aks-app-routing-operator/pull/365)
- only installs needed CRDs - [link](https://github.com/Azure/aks-app-routing-operator/pull/383)
- add disable expensive cache - [link](https://github.com/Azure/aks-app-routing-operator/pull/391)

### Changed
- switches containers to use MCR Go - [link](https://github.com/Azure/aks-app-routing-operator/pull/254)
- bump nginx to 1.11.5 - [link](https://github.com/Azure/aks-app-routing-operator/pull/270), [link2](https://github.com/Azure/aks-app-routing-operator/pull/402)
- move prom port to new cluster ip service - [link](https://github.com/Azure/aks-app-routing-operator/pull/295)
- ignore conflict errors for NginxIngressController status updates - [link](https://github.com/Azure/aks-app-routing-operator/pull/388)

### Fixed
- add is active check to watchdog before erroring - [link](https://github.com/Azure/aks-app-routing-operator/pull/357)

## [0.2.3-patch-6] - 2025-03-25

### Changed
- bump ingress-nginx to v1.11.5 - [link](https://github.com/Azure/aks-app-routing-operator/pull/404)

## [0.2.1-patch-8] - 2025-03-25

### Changed
- bump ingress-nginx to v1.11.5 - [link](https://github.com/Azure/aks-app-routing-operator/pull/403)

## [0.2.3-patch-5] - 2025-01-07

### Changed
 - bump crypto to v0.31.0 - [link](https://github.com/Azure/aks-app-routing-operator/pull/329)
 - bump net to v0.33.0 - [link](https://github.com/Azure/aks-app-routing-operator/pull/329)

## [0.2.1-patch-7] - 2025-01-07

### Changed
 - bump crypto to v0.31.0 - [link](https://github.com/Azure/aks-app-routing-operator/pull/330)
 - bump net to v0.33.0 - [link](https://github.com/Azure/aks-app-routing-operator/pull/330)

## [0.2.3-patch-4] - 2024-12-06

### Changed

- bump Go - [link](https://github.com/Azure/aks-app-routing-operator/pull/315)
- bump externaldns to v0.15.0 - [link](https://github.com/Azure/aks-app-routing-operator/pull/315)

## [0.2.1-patch-6] - 2024-12-06

### Changed

- bump Go - [link](https://github.com/Azure/aks-app-routing-operator/pull/316)
- bump externaldns to v0.15.0 - [link](https://github.com/Azure/aks-app-routing-operator/pull/316)

## [0.2.3-patch-3] - 2024-10-24

### Changed

- bump Go - [link](https://github.com/Azure/aks-app-routing-operator/pull/303)
- split out prom service - [link](https://github.com/Azure/aks-app-routing-operator/pull/303)


## [0.2.1-patch-5] - 2024-10-14

### Changed

- patch sec vulns and backport osm changes - [link](https://github.com/Azure/aks-app-routing-operator/pull/284)
- split out prom service - [link](https://github.com/Azure/aks-app-routing-operator/commit/b9a6cb0122d3af608a217accb4de4c5023e5809d)

## [0.2.3-patch-2] - 2024-08-17

### Changed

- bumped ingress-nginx version to 1.11.2 - [link](https://github.com/Azure/aks-app-routing-operator/pull/272)

## [0.2.1-patch-4] - 2024-08-17

### Changed

- bumped ingress-nginx version to 1.11.2 - [link](https://github.com/Azure/aks-app-routing-operator/pull/271)

## [0.2.1-patch-3] - 2024-07-31

### Changed

- bumped ExternalDNS version to 0.13.5 - [link](https://github.com/Azure/aks-app-routing-operator/commit/739dda16134ca65d44d3d438352e9e9dbab1cc89)

## [0.2.3-patch-1] - 2024-07-10

### Fixed 

- PlaceholderPod cleanup doesn't exit - [link](https://github.com/Azure/aks-app-routing-operator/commit/7375a2536f7fa9e88091640a1fc9d77827db800c)

## [0.2.4-patch-1] - 2024-07-10

### Fixed 

- PlaceholderPod cleanup doesn't exit - [link](https://github.com/Azure/aks-app-routing-operator/commit/895cb29936c4427cddcb3c0637b8de381c3a9673)

## [0.2.1-patch-2] - 2024-07-10

### Changed

- Added `non-root: true` to Nginx security context - [link](https://github.com/Azure/aks-app-routing-operator/commit/2d973d5fe4e70b9646cb0e2a358a0e36a6385872)

### Fixed

- PlaceholderPod cleanup doesn't exit - [link](https://github.com/Azure/aks-app-routing-operator/commit/503c2e7b2116beccc0902cc59f64b8c2d3d484fc)

## [0.2.1-patch-1] - 2024-05-16

### Changed

- bumped Nginx version to 1.10 - [link](https://github.com/Azure/aks-app-routing-operator/commit/57a721bf4ef41e821a317a3ad0fc201da9c15827)

## [0.2.4] - 2024-05-09

### Added

- force ssl redirect configuration - [#173](https://github.com/Azure/aks-app-routing-operator/pull/173)

### Changed

- swap default http/s ports [#202](https://github.com/Azure/aks-app-routing-operator/pull/202)

## [0.2.3] - 2024-04-22

### Added

- add security context to placeholder pod - [#195](https://github.com/Azure/aks-app-routing-operator/pull/195)
- add security context to external dns - [#194](https://github.com/Azure/aks-app-routing-operator/pull/194)

### Changed

- bumped external dns to 0.13.5 - [#196](https://github.com/Azure/aks-app-routing-operator/pull/196)
- harden nginx security context - [#192](https://github.com/Azure/aks-app-routing-operator/pull/192)


## [0.2.2] - 2024-04-03

### Added

- default SSL certificate secret configuration through CRD - [#160](https://github.com/Azure/aks-app-routing-operator/pull/160)
- min and max replica configuration through CRD - [#178](https://github.com/Azure/aks-app-routing-operator/pull/178)
- scaling threshold configuration through CRD - [#180](https://github.com/Azure/aks-app-routing-operator/pull/180)
- tls reconciler that allows us to manage Ingress TLS fields - [#155](https://github.com/Azure/aks-app-routing-operator/pull/155)
- bumped to Nginx 1.10.0 - [#184](https://github.com/Azure/aks-app-routing-operator/pull/184)
- default SSL certificate keyvault uri configuration through CRD - [#166](https://github.com/Azure/aks-app-routing-operator/pull/166)
- liveness probe to Nginx - [#188](https://github.com/Azure/aks-app-routing-operator/pull/188)
- default NginxIngressController configuration through CLI args - [#189](https://github.com/Azure/aks-app-routing-operator/pull/189)

### Changes

- switched to distroless base image - [#164](https://github.com/Azure/aks-app-routing-operator/pull/164)
- changed OSM integration to disable sidecar injection on Nginx deployment - [#170](https://github.com/Azure/aks-app-routing-operator/pull/170)

## [0.2.1] - 2024-01-23

### Fixed

- Respect immutable fields in Placeholder Pod reconciler - [#153](https://github.com/Azure/aks-app-routing-operator/pull/153)

## [0.2.0] - 2024-01-11

### Changes

- Slightly lowered priority class to system cluster critical - [#148](https://github.com/Azure/aks-app-routing-operator/pull/148)

### Added

- Log for number of target Nginx replicas - [#144](https://github.com/Azure/aks-app-routing-operator/pull/144)
- CRD YAML definition-based validation and defaults - [#150](https://github.com/Azure/aks-app-routing-operator/pull/150)

### Removed

- Webhooks and webhook logic - [#151](https://github.com/Azure/aks-app-routing-operator/pull/151)

## [0.1.2] - 2023-11-28

### Changes

- Make DNS Zone resource group check case insensitive - [#137](https://github.com/Azure/aks-app-routing-operator/pull/137)
- Make Default NginxIngressController always reconcile - [#138](https://github.com/Azure/aks-app-routing-operator/pull/138)
- Expose useful function for interacting with NginxIngressController publicly - [#140](https://github.com/Azure/aks-app-routing-operator/pull/140), [#141](https://github.com/Azure/aks-app-routing-operator/pull/141), [#142](https://github.com/Azure/aks-app-routing-operator/pull/142)

## [0.1.1] - 2023-11-12

### Added

- Add lowercase validation to NginxIngressController CRD - [#136](https://github.com/Azure/aks-app-routing-operator/pull/136)

## [0.1.0] - 2023-11-11

### Added

- Add NginxIngressController CRD - [#121](https://github.com/Azure/aks-app-routing-operator/pull/121)
- Apply CRDs on operator startup - [#122](https://github.com/Azure/aks-app-routing-operator/pull/122)
- Add CRD Webhooks - [#123](https://github.com/Azure/aks-app-routing-operator/pull/123)
- Add prom metrics to Webhooks - [#125](https://github.com/Azure/aks-app-routing-operator/pull/125)
- Add CRD reconciler - [#124](https://github.com/Azure/aks-app-routing-operator/pull/124)
- Add default NginxIngressController reconciler - [#126](https://github.com/Azure/aks-app-routing-operator/pull/126)
- Switch all controllers to use CRD as source of truth - [#128](https://github.com/Azure/aks-app-routing-operator/pull/128)
- Add new Ingress events - [#130](https://github.com/Azure/aks-app-routing-operator/pull/130)
- Add ownership reference to webhook config - [#129](https://github.com/Azure/aks-app-routing-operator/pull/129)
- Generate Webhook certs - [#131](https://github.com/Azure/aks-app-routing-operator/pull/131)
- Use dns hostname for cert - [#133](https://github.com/Azure/aks-app-routing-operator/pull/133)
- Add E2e tests for CRD - [#132](https://github.com/Azure/aks-app-routing-operator/pull/132)

## [0.0.7] - 2023-11-04

### Changed

- Revert label selector on Placeholder pod to be backwards compatible

## [0.0.6] - 2023-10-27

### Added

- Removed resource limtis and add topology spread - [#119](https://github.com/Azure/aks-app-routing-operator/pull/119)
- Add managed-by label to all managed resource and check for that before cleaning - [#111](https://github.com/Azure/aks-app-routing-operator/pull/111)

### Changed

- Upgrade NGINX Ingress Controller to v1.9.4 - [#118](https://github.com/Azure/aks-app-routing-operator/pull/118)

## [0.0.5] - 2023-10-13

### Added

- Improved logging across entire operator - [#110](https://github.com/Azure/aks-app-routing-operator/pull/110)

### Changed

- Upgrade NGINX Ingress Controller to v1.8.4 - [#113](https://github.com/Azure/aks-app-routing-operator/pull/113)

## [0.0.4] - 2023-10-05

### Added

- Improved error logging - [#97](https://github.com/Azure/aks-app-routing-operator/pull/97)
- Improved E2E testing framework that tests upgrade story and all operator configurations - [#79](https://github.com/Azure/aks-app-routing-operator/pull/79), [#90](https://github.com/Azure/aks-app-routing-operator/pull/90), [#95](https://github.com/Azure/aks-app-routing-operator/pull/95), [#98](https://github.com/Azure/aks-app-routing-operator/pull/98), [#100](https://github.com/Azure/aks-app-routing-operator/pull/100), [#104](https://github.com/Azure/aks-app-routing-operator/pull/104)

### Changed

- Bump dependencies - [#92](https://github.com/Azure/aks-app-routing-operator/pull/92)
- Upgrade NGINX Ingress Controller to v1.8.1 - [#89](https://github.com/Azure/aks-app-routing-operator/pull/89)

## [0.0.3] - 2023-08-25

### Added

- Leader election and operator multi-replica support (adds resilliency through multliple replicas and PDB) - [#64](https://github.com/Azure/aks-app-routing-operator/pull/64)
- Switch Operator logging to Zap for JSON structured logging - [#69](https://github.com/Azure/aks-app-routing-operator/pull/69)
- Add filename and line numbers to logs - [#72](https://github.com/Azure/aks-app-routing-operator/pull/72)
- Add Unit test coverage checks and validation to repository - [#74](https://github.com/Azure/aks-app-routing-operator/pull/74)
- Add CodeQL security scanning to repository - [#70](https://github.com/Azure/aks-app-routing-operator/pull/70)
- Add Prometheus metrics for reconciliation loops total and errors - [#76](https://github.com/Azure/aks-app-routing-operator/pull/76)
- Increase unit test coverage - [#77](https://github.com/Azure/aks-app-routing-operator/pull/77), [#82](https://github.com/Azure/aks-app-routing-operator/pull/82)
- Add controller name structure to each controller so logs and metrics have consistient and related controller names - [#80](https://github.com/Azure/aks-app-routing-operator/pull/80), [#84](https://github.com/Azure/aks-app-routing-operator/pull/84), [#85](https://github.com/Azure/aks-app-routing-operator/pull/85), [#86](https://github.com/Azure/aks-app-routing-operator/pull/86)

## [0.0.2] - 2023-07-10

### Fixed

- IngressClass Controller field immutable bug

## [0.0.1] - 2022-05-24

### Added

- Initial release of App Routing 🚢
