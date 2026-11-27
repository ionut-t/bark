# Changelog

## [2.40.0](https://github.com/ionut-t/bark/compare/v2.39.1...v2.40.0) (2026-11-27)


### Features

* **enclosing:** prioritise densest declarations when capping diff context ([#200](https://github.com/ionut-t/bark/issues/200)) ([d0c307b](https://github.com/ionut-t/bark/commit/d0c307ba74bafa443371cbd1da592ed847fa984e))
* **enclosing:** report dropped declarations exceeding line cap ([#198](https://github.com/ionut-t/bark/issues/198)) ([948501f](https://github.com/ionut-t/bark/commit/948501f40f8e6e7c7b2be429ec1107f171180033))
* **review:** add random selection of preferred reviewers ([#202](https://github.com/ionut-t/bark/issues/202)) ([f808da3](https://github.com/ionut-t/bark/commit/f808da3dadd5a98a3db0e3d5ccf6358b883e7060))
* **review:** enable context enrichment by default ([#203](https://github.com/ionut-t/bark/issues/203)) ([393064a](https://github.com/ionut-t/bark/commit/393064af3c3029d32395a1d32f0cad5bd46cbb60))


### Bug Fixes

* **enclosing:** group labels for shared line declarations ([#201](https://github.com/ionut-t/bark/issues/201)) ([9d8c92c](https://github.com/ionut-t/bark/commit/9d8c92c444082d6e4c4389fb4049f5704d1a3b5a))

## [2.39.1](https://github.com/ionut-t/bark/compare/v2.39.0...v2.39.1) (2026-11-26)


### Bug Fixes

* **debuglog:** ensure unique filenames for same-millisecond logs ([#192](https://github.com/ionut-t/bark/issues/192)) ([43a159d](https://github.com/ionut-t/bark/commit/43a159d64347e14e7d47c565281e8345332b5f33))
* **enclosing:** filter qualified calls to imported packages ([#197](https://github.com/ionut-t/bark/issues/197)) ([6fdc707](https://github.com/ionut-t/bark/commit/6fdc70749ea7b836c483a11a4e69cd8c6d2cdcd7))


### Performance Improvements

* **enclosing:** skip declarations consisting entirely of added lines ([#196](https://github.com/ionut-t/bark/issues/196)) ([a2915d4](https://github.com/ionut-t/bark/commit/a2915d4bd7efccb31aa84d55e86c1c7b386ac39a))

## [2.39.0](https://github.com/ionut-t/bark/compare/v2.38.0...v2.39.0) (2026-11-24)


### Features

* **ci:** allow selecting reviewer via pull request label ([#189](https://github.com/ionut-t/bark/issues/189)) ([31964fe](https://github.com/ionut-t/bark/commit/31964fe6cbd478f473b7563f0ace29fb9b182a9c))
* **reviewers:** add Billy Butcher and Homelander persona prompts ([#191](https://github.com/ionut-t/bark/issues/191)) ([02cf664](https://github.com/ionut-t/bark/commit/02cf664eb0fb47ae3da4ac9edbcbbd254c57de6a))
* **reviewers:** add new fictional character personas ([#188](https://github.com/ionut-t/bark/issues/188)) ([3d465e4](https://github.com/ionut-t/bark/commit/3d465e4a632dec270ef244c36ba25f67453a156c))


### Bug Fixes

* **git:** allow reviewing changes in repositories without commits ([#187](https://github.com/ionut-t/bark/issues/187)) ([11d64ef](https://github.com/ionut-t/bark/commit/11d64eff7b83600473b288dda0664eef5938d393))
* **review:** handle changes containing only ignored files ([#183](https://github.com/ionut-t/bark/issues/183)) ([3218486](https://github.com/ionut-t/bark/commit/321848617f2130f1576eede8cbfa316561736b9f))
* **tui:** ensure consistent list sizing on window resize ([#190](https://github.com/ionut-t/bark/issues/190)) ([2424035](https://github.com/ionut-t/bark/commit/242403575920540532b8e4eac757fb59be3c660f))
* **tui:** handle empty review diffs gracefully ([#185](https://github.com/ionut-t/bark/issues/185)) ([49516e9](https://github.com/ionut-t/bark/commit/49516e91baa5be9abd0e9b15ac48e9b38fa4eb25))
* **tui:** manage staged-only flag during review selection ([#186](https://github.com/ionut-t/bark/issues/186)) ([1e61650](https://github.com/ionut-t/bark/commit/1e616502bd4a1422e41b09ee0268fcbea6d3abcb))

## [2.38.0](https://github.com/ionut-t/bark/compare/v2.37.0...v2.38.0) (2026-11-24)


### Features

* **enclosing:** implement Python module resolution for imports ([#181](https://github.com/ionut-t/bark/issues/181)) ([78d34ff](https://github.com/ionut-t/bark/commit/78d34ff6d652f583022d01ab891d76034fbd79a3))
* **review:** add dry-run mode to review prompts ([#178](https://github.com/ionut-t/bark/issues/178)) ([5351f7e](https://github.com/ionut-t/bark/commit/5351f7ef8aeaa287af69a554884914832a4f7f14))


### Bug Fixes

* **cli:** improve stdin pipe behaviour and prevent hangs ([#182](https://github.com/ionut-t/bark/issues/182)) ([bf64657](https://github.com/ionut-t/bark/commit/bf646579b947218fa530c399e5a8252781d8de02))
* **enclosing:** prefer definitions outside .barkignore when a name is ambiguous ([#180](https://github.com/ionut-t/bark/issues/180)) ([a3e7b60](https://github.com/ionut-t/bark/commit/a3e7b60547b416440a5939c56ea7f996c80e8c1c))

## [2.37.0](https://github.com/ionut-t/bark/compare/v2.36.0...v2.37.0) (2026-11-23)


### Features

* **config:** add configurable timeout for model calls ([#175](https://github.com/ionut-t/bark/issues/175)) ([0e8c1f4](https://github.com/ionut-t/bark/commit/0e8c1f402d3e37e2ed93ce5f1517e6921fd45634))


### Bug Fixes

* **enclosing:** include type definitions and resolve TS imports ([#177](https://github.com/ionut-t/bark/issues/177)) ([c85e381](https://github.com/ionut-t/bark/commit/c85e381a95f7c6b922115a952e8a3e4cdabd2939))

## [2.36.0](https://github.com/ionut-t/bark/compare/v2.35.1...v2.36.0) (2026-11-22)


### Features

* **redact:** implement secret masking and security reporting ([#169](https://github.com/ionut-t/bark/issues/169)) ([cb8bb99](https://github.com/ionut-t/bark/commit/cb8bb9998a9c75804a647415b185e8c8c4cdedcd))


### Bug Fixes

* **actions:** synchronise workflow selection and improve editor manag… ([#173](https://github.com/ionut-t/bark/issues/173)) ([5312b95](https://github.com/ionut-t/bark/commit/5312b9582e51952e27d2a2c811d188986a22f56b))
* **tui:** use pointer receivers for setSize methods ([#174](https://github.com/ionut-t/bark/issues/174)) ([f23f050](https://github.com/ionut-t/bark/commit/f23f0503ba6564b718118c9f5e103a483e652131))

## [2.35.1](https://github.com/ionut-t/bark/compare/v2.35.0...v2.35.1) (2026-11-22)


### Bug Fixes

* **check:** confine finding paths to files in the diff ([#166](https://github.com/ionut-t/bark/issues/166)) ([20f6385](https://github.com/ionut-t/bark/commit/20f6385676aeb6e2bec2a35208fa8ec7b8d0528e))
* **prreview:** use the recorded finding range for review threads ([#165](https://github.com/ionut-t/bark/issues/165)) ([3b3a3de](https://github.com/ionut-t/bark/commit/3b3a3deb9e77c8b0330bf7cefa2b231ac5394ed9))
* **review:** prevent auto-approval when configuration files change ([#163](https://github.com/ionut-t/bark/issues/163)) ([18e06a4](https://github.com/ionut-t/bark/commit/18e06a49a480a14d8cc177c503f65a972a1c69bf))

## [2.35.0](https://github.com/ionut-t/bark/compare/v2.34.1...v2.35.0) (2026-11-21)


### Features

* **ci:** harden release process and add build provenance ([#162](https://github.com/ionut-t/bark/issues/162)) ([37fd62c](https://github.com/ionut-t/bark/commit/37fd62c613559c469ce6848bf94f5464fadb90d5))


### Bug Fixes

* **check:** treat doubted findings as new instead of repeats ([#161](https://github.com/ionut-t/bark/issues/161)) ([3c7a32a](https://github.com/ionut-t/bark/commit/3c7a32a43f5e04a72c43f101d0baae1f0bac89a5))
* **enclosing:** ignore interface method signatures in caller identification ([#158](https://github.com/ionut-t/bark/issues/158)) ([3c18709](https://github.com/ionut-t/bark/commit/3c18709664c7affa5dea86f28cefbc6b738ffd05))
* **enclosing:** match Go function callers by package ([#160](https://github.com/ionut-t/bark/issues/160)) ([0f7c340](https://github.com/ionut-t/bark/commit/0f7c340b49754c92305653c30a87816fe1af1aa3))

## [2.34.1](https://github.com/ionut-t/bark/compare/v2.34.0...v2.34.1) (2026-11-21)


### Bug Fixes

* **cli:** wrap help flag descriptions to fit terminal width ([#156](https://github.com/ionut-t/bark/issues/156)) ([befc986](https://github.com/ionut-t/bark/commit/befc9862b791dbd00d83aa4d785ddfe4952944d6))

## [2.34.0](https://github.com/ionut-t/bark/compare/v2.33.0...v2.34.0) (2026-11-21)


### Features

* **review:** add automatic pull request approval support ([#154](https://github.com/ionut-t/bark/issues/154)) ([9452c95](https://github.com/ionut-t/bark/commit/9452c959dd3539668cd0ed715bc584aeb7c1b1da))
* **review:** automatically resolve fixed review threads ([#155](https://github.com/ionut-t/bark/issues/155)) ([2dd2577](https://github.com/ionut-t/bark/commit/2dd2577aa2d3bc5fa003db533da33e34b7b64d21))


### Bug Fixes

* **check:** handle empty or partial LLM answers ([#151](https://github.com/ionut-t/bark/issues/151)) ([201833a](https://github.com/ionut-t/bark/commit/201833a02b9a8c903e2ea9994193c845f2dedfcc))
* **findings:** mark findings as unchecked when validation provides no verdict ([#153](https://github.com/ionut-t/bark/issues/153)) ([9ed60ce](https://github.com/ionut-t/bark/commit/9ed60ce23356c99b2007b7b30f39a1393afd44a5))

## [2.33.0](https://github.com/ionut-t/bark/compare/v2.32.0...v2.33.0) (2026-11-20)


### Features

* **llm:** add effort setting to control model thinking ([#150](https://github.com/ionut-t/bark/issues/150)) ([5e759e2](https://github.com/ionut-t/bark/commit/5e759e2d1beb180568618657cffc8cf70f14e111))
* **review:** add support for repository-specific house rules ([#147](https://github.com/ionut-t/bark/issues/147)) ([e0bd195](https://github.com/ionut-t/bark/commit/e0bd19594d649bc208f561f1fbbbd8761a62dabd))

## [2.32.0](https://github.com/ionut-t/bark/compare/v2.31.0...v2.32.0) (2026-11-19)


### Features

* **prreview:** stop recording repeated findings ([#144](https://github.com/ionut-t/bark/issues/144)) ([c0634ef](https://github.com/ionut-t/bark/commit/c0634efd26472d9d4bcd9a61624d78aedd2d3202))
* **review:** add --fail-on to fail a review on severe findings ([ee67ac9](https://github.com/ionut-t/bark/commit/ee67ac96d491c7028c6b018e0a87e3f3299af073))
* **review:** implement incremental pull request reviews ([#140](https://github.com/ionut-t/bark/issues/140)) ([e9fe3a5](https://github.com/ionut-t/bark/commit/e9fe3a53df30677529d589d13b019146b210703e))
* **review:** keep core code first when cutting large diffs ([#146](https://github.com/ionut-t/bark/issues/146)) ([797d531](https://github.com/ionut-t/bark/commit/797d5311ae18e81ab84ed071e6f2d227942bfc98))
* **review:** skip posting reviews with no new findings ([#145](https://github.com/ionut-t/bark/issues/145)) ([c8d672a](https://github.com/ionut-t/bark/commit/c8d672a743fabf7a86e9b6164f868a4187a84028))
* **review:** support dismissing findings via reactions or replies ([#143](https://github.com/ionut-t/bark/issues/143)) ([5b68798](https://github.com/ionut-t/bark/commit/5b687985e1aee09546f5b65726d823c6860c5fc8))


### Bug Fixes

* **review:** keep findings raised before as open in the rewritten verdict ([ee67ac9](https://github.com/ionut-t/bark/commit/ee67ac96d491c7028c6b018e0a87e3f3299af073))

## [2.31.0](https://github.com/ionut-t/bark/compare/v2.30.0...v2.31.0) (2026-11-18)


### Features

* **config:** support XDG base directories and migration ([#133](https://github.com/ionut-t/bark/issues/133)) ([09d6e54](https://github.com/ionut-t/bark/commit/09d6e54ac196492b618f5cb9a359e69946e43a2c))
* **git:** add token-based diff limiting and deprecate line limits ([#131](https://github.com/ionut-t/bark/issues/131)) ([35cabfa](https://github.com/ionut-t/bark/commit/35cabfa1befd271272a104bc9fb11c43e5945f24))


### Bug Fixes

* **actions:** pass inputs via environment variables ([#134](https://github.com/ionut-t/bark/issues/134)) ([601b421](https://github.com/ionut-t/bark/commit/601b421bf3d2bacf464c93808abdcdae8bbd8656))
* **ci:** paginate review comment lookup to prevent duplicates ([#135](https://github.com/ionut-t/bark/issues/135)) ([357b7f9](https://github.com/ionut-t/bark/commit/357b7f959a0b332f86bf4f355bfcdad1f6a65168))
* **git:** enhance pull request context and diff fallback handling ([#138](https://github.com/ionut-t/bark/issues/138)) ([44ae089](https://github.com/ionut-t/bark/commit/44ae089a67aa5c214e5141ccc910ea132a159b47))
* **git:** report truncated diffs in code reviews ([#137](https://github.com/ionut-t/bark/issues/137)) ([a7b563c](https://github.com/ionut-t/bark/commit/a7b563c86c356e0de456bf40b1e7f2f9b0388b45))
* **llm:** detect and signal truncated model output ([#136](https://github.com/ionut-t/bark/issues/136)) ([2771b4d](https://github.com/ionut-t/bark/commit/2771b4d57c257c7ca55f5f19736a898d1b85bcf2))
* **tui:** show loading spinner while preparing data ([#139](https://github.com/ionut-t/bark/issues/139)) ([023e93d](https://github.com/ionut-t/bark/commit/023e93d0abff7c62806e3b985839908b2ce336a6))

## [2.30.0](https://github.com/ionut-t/bark/compare/v2.29.1...v2.30.0) (2026-11-15)


### Features

* **instructions:** allow combining several instructions ([#123](https://github.com/ionut-t/bark/issues/123)) ([ab53b02](https://github.com/ionut-t/bark/commit/ab53b023dbf864d9723e94d720502f2387dbafbd))
* **tui:** add British-themed loading messages and standardise spelling ([#127](https://github.com/ionut-t/bark/issues/127)) ([0b3c3db](https://github.com/ionut-t/bark/commit/0b3c3dbed8a23c9188acef592170377152331d71))
* **tui:** mark findings in-place after automated check ([#126](https://github.com/ionut-t/bark/issues/126)) ([e0b9c6a](https://github.com/ionut-t/bark/commit/e0b9c6af9b129fb0f57391f5093268330d5f2e0f))


### Bug Fixes

* **cmd:** hide unknown metadata from version banner ([#129](https://github.com/ionut-t/bark/issues/129)) ([235ced9](https://github.com/ionut-t/bark/commit/235ced9e2bae55db592d50b6c06693b306d3441a))
* **tui:** ensure commit retry reloads repository state ([#125](https://github.com/ionut-t/bark/issues/125)) ([da93873](https://github.com/ionut-t/bark/commit/da9387366443229d09080ad22ff421fcd0207246))

## [2.29.1](https://github.com/ionut-t/bark/compare/v2.29.0...v2.29.1) (2026-11-12)


### Bug Fixes

* **tui:** prevent blocking terminal background colour queries ([#120](https://github.com/ionut-t/bark/issues/120)) ([a32380f](https://github.com/ionut-t/bark/commit/a32380f728e8c12131a37c3b08a686d38d6e7d4c))

## [2.29.0](https://github.com/ionut-t/bark/compare/v2.28.0...v2.29.0) (2026-11-12)


### Features

* **llm:** infer provider from model name ([#117](https://github.com/ionut-t/bark/issues/117)) ([9a8b87f](https://github.com/ionut-t/bark/commit/9a8b87f8268334131d98f141aec00c4f41db680f))


### Bug Fixes

* **templates:** remove issue references in commit messages ([#119](https://github.com/ionut-t/bark/issues/119)) ([6677a71](https://github.com/ionut-t/bark/commit/6677a710c6a5dd9354d49435e08d10461aa15962))

## [2.28.0](https://github.com/ionut-t/bark/compare/v2.27.1...v2.28.0) (2026-11-12)


### Features

* **commit:** include branch name in prompt context ([#116](https://github.com/ionut-t/bark/issues/116)) ([482ecc7](https://github.com/ionut-t/bark/commit/482ecc7ac84f23f61e4361d1d8d6620375560bd8))
* **config:** add repository-level settings and barkignore ([#113](https://github.com/ionut-t/bark/issues/113)) ([84ea582](https://github.com/ionut-t/bark/commit/84ea5825af67ae3b8b69998b40336cb721ef2e4a))

## [2.27.1](https://github.com/ionut-t/bark/compare/v2.27.0...v2.27.1) (2026-11-10)


### Bug Fixes

* **anthropic:** read all content blocks in Generate ([#111](https://github.com/ionut-t/bark/issues/111)) ([ebf26fb](https://github.com/ionut-t/bark/commit/ebf26fb8c5faed938824992078a9bb9ccc2f73f2))

## [2.27.0](https://github.com/ionut-t/bark/compare/v2.26.0...v2.27.0) (2026-11-09)


### Features

* **check:** set aside findings raised before on pull requests ([#108](https://github.com/ionut-t/bark/issues/108)) ([1d02521](https://github.com/ionut-t/bark/commit/1d02521e2f9f489afdcfd15a5e7d5b9cfec7a3d6))
* **costs:** add model pricing and cost tracking ([#109](https://github.com/ionut-t/bark/issues/109)) ([f0782b9](https://github.com/ionut-t/bark/commit/f0782b9c98e1efcd15f15b06091788302cdfd4f8))
* **review:** add support for posting inline PR comments ([#104](https://github.com/ionut-t/bark/issues/104)) ([11b4b9a](https://github.com/ionut-t/bark/commit/11b4b9adcc75c8b09839a3c482b807a7e8a44945))
* **review:** add support for structured findings ([#102](https://github.com/ionut-t/bark/issues/102)) ([91a1ddd](https://github.com/ionut-t/bark/commit/91a1ddd39da5db11af7960589c7b47d678ca67cb))
* **review:** check structured findings against the changed files ([#105](https://github.com/ionut-t/bark/issues/105)) ([beb0b20](https://github.com/ionut-t/bark/commit/beb0b204b2a72070c2c465d92a42ddc861491369))
* **review:** include related code context in review context ([#107](https://github.com/ionut-t/bark/issues/107)) ([67134d0](https://github.com/ionut-t/bark/commit/67134d0826b1b8905de6d27e6171295f35f39a90))


### Bug Fixes

* **cmd:** restrict --plain flag to relevant subcommands ([#110](https://github.com/ionut-t/bark/issues/110)) ([9144737](https://github.com/ionut-t/bark/commit/914473751ec38aba9f5f04d18ec26f58ad500010))

## [2.26.0](https://github.com/ionut-t/bark/compare/v2.25.1...v2.26.0) (2026-11-08)


### Features

* **config:** improve command flags for consistency ([#94](https://github.com/ionut-t/bark/issues/94)) ([692efde](https://github.com/ionut-t/bark/commit/692efde45920292b5798427b4b718d95b9fa2316))


### Bug Fixes

* **cli:** ensure non-zero exit code on command failure ([#90](https://github.com/ionut-t/bark/issues/90)) ([3dff236](https://github.com/ionut-t/bark/commit/3dff236bd80ce328e1d636654f3e319f5a3f2880))
* **config:** load default values for missing config keys ([#99](https://github.com/ionut-t/bark/issues/99)) ([1a1c50e](https://github.com/ionut-t/bark/commit/1a1c50ee93cf4f399faee2e3bcf94ac51c2514a1))
* **config:** make --config load the given config file ([#97](https://github.com/ionut-t/bark/issues/97)) ([fdc46a1](https://github.com/ionut-t/bark/commit/fdc46a1392913bad08df441abad5cfec337d3ec5))
* **config:** never save one-run overrides to the config file ([#96](https://github.com/ionut-t/bark/issues/96)) ([89323aa](https://github.com/ionut-t/bark/commit/89323aa1d466a457b5c42c5d51794965f65e11d4))
* **config:** prevent data race on config access ([#98](https://github.com/ionut-t/bark/issues/98)) ([326fee0](https://github.com/ionut-t/bark/commit/326fee02f9e73807407d2214b81a5d0a45c64208))
* **git:** handle base branch names containing slashes ([#101](https://github.com/ionut-t/bark/issues/101)) ([3473345](https://github.com/ionut-t/bark/commit/34733453f56c45f35b1855c111e0b9e98c5cdcab))
* **llm:** prevent deadlock when stream and close both error ([#92](https://github.com/ionut-t/bark/issues/92)) ([df8ed33](https://github.com/ionut-t/bark/commit/df8ed33f0f8adf7344d116a824a59f7a44fbb8bc))

## [2.25.1](https://github.com/ionut-t/bark/compare/v2.25.0...v2.25.1) (2026-11-07)


### Bug Fixes

* **config:** ensure empty provider override respects configured value ([#83](https://github.com/ionut-t/bark/issues/83)) ([deed8e2](https://github.com/ionut-t/bark/commit/deed8e2a026a583f629b03f350f3e8c9336306a3))
* **git:** fall back to common branch names for base branch detection ([#88](https://github.com/ionut-t/bark/issues/88)) ([66b0f7e](https://github.com/ionut-t/bark/commit/66b0f7effbc6d713c0a85bf760df421e2666dee0))
* **git:** include untracked files in working tree diff ([#87](https://github.com/ionut-t/bark/issues/87)) ([99f16ae](https://github.com/ionut-t/bark/commit/99f16ae9d78ba30ddfae25554f8180056f4f6a6e))
* **git:** use merge-base for branch diffs ([#86](https://github.com/ionut-t/bark/issues/86)) ([e257b93](https://github.com/ionut-t/bark/commit/e257b930096368799689e7752b8bd99fcd21ea21))
* **git:** use NUL delimiters for robust commit parsing ([#89](https://github.com/ionut-t/bark/issues/89)) ([69d6c73](https://github.com/ionut-t/bark/commit/69d6c73c9f6580ebd75aa664fcd2c104fbbf5bec))

## [2.25.0](https://github.com/ionut-t/bark/compare/v2.24.1...v2.25.0) (2026-11-06)


### Features

* **tui:** enable cursor movement across lines in editor ([#81](https://github.com/ionut-t/bark/issues/81)) ([27fd563](https://github.com/ionut-t/bark/commit/27fd563235d0b928b21f2d8e9c120323e1b64172))

## [2.24.1](https://github.com/ionut-t/bark/compare/v2.24.0...v2.24.1) (2026-10-08)


### Bug Fixes

* **tui:** change regenerate keybinding from ctrl+r to ctrl+g ([#76](https://github.com/ionut-t/bark/issues/76)) ([bcd3610](https://github.com/ionut-t/bark/commit/bcd36108ea828ba664d5110894b67808c874de44))

## [2.24.0](https://github.com/ionut-t/bark/compare/v2.23.0...v2.24.0) (2026-09-02)


### Miscellaneous Chores

* release 2.24.0 ([fc8a987](https://github.com/ionut-t/bark/commit/fc8a987eb95f886f98290061eca2e5c38fd6a090))

## [2.23.0](https://github.com/ionut-t/bark/compare/v2.22.0...v2.23.0) (2026-07-12)


### Features

* **llm:** track and display token usage statistics ([#72](https://github.com/ionut-t/bark/issues/72)) ([31f472c](https://github.com/ionut-t/bark/commit/31f472c2f2353f0181d3b3da9f755ca86632a341))
* **review:** enrich prompts with enclosing code context ([#69](https://github.com/ionut-t/bark/issues/69)) ([69b0c17](https://github.com/ionut-t/bark/commit/69b0c177cd1bf3b2f14b0f774e13714dd8634236))


### Bug Fixes

* **tui:** coalesce stream deltas to prevent UI lag and flicker ([#71](https://github.com/ionut-t/bark/issues/71)) ([f0fcc3f](https://github.com/ionut-t/bark/commit/f0fcc3fb1bd94508b9cfedc5bb1b0cd74410c17a))

## [2.22.0](https://github.com/ionut-t/bark/compare/v2.21.0...v2.22.0) (2026-06-21)


### Features

* **review:** enrich review context ([#64](https://github.com/ionut-t/bark/issues/64)) ([6dc22bc](https://github.com/ionut-t/bark/commit/6dc22bc601ab6e1920695af0ffa531f2a2e1de3d))

## [2.21.0](https://github.com/ionut-t/bark/compare/v2.20.0...v2.21.0) (2026-06-19)


### Features

* **cli:** add flags to override model and provider for the commit cmd ([#63](https://github.com/ionut-t/bark/issues/63)) ([c1098f1](https://github.com/ionut-t/bark/commit/c1098f1e2408c50e73468ab99d593ba51b2fbb6b))
* **llm:** add Anthropic provider support ([#62](https://github.com/ionut-t/bark/issues/62)) ([824167b](https://github.com/ionut-t/bark/commit/824167bddc3393b2f66670a9d8c1bc4838c9e2b4))
* **llm:** add Ollama support ([#60](https://github.com/ionut-t/bark/issues/60)) ([69db58a](https://github.com/ionut-t/bark/commit/69db58aca54d618a8922e4b1947894d38af0322e))
* **tui:** display LLM model in editor status line ([#58](https://github.com/ionut-t/bark/issues/58)) ([8c1b451](https://github.com/ionut-t/bark/commit/8c1b4513f56af5ecef247147718954e4e9d33888))


### Bug Fixes

* **tui:** change help key to f1 to resolve search conflict ([#61](https://github.com/ionut-t/bark/issues/61)) ([a1be5b1](https://github.com/ionut-t/bark/commit/a1be5b1adeab91556f6dd9cb2330ba566e9cf3f4))

## [2.20.0](https://github.com/ionut-t/bark/compare/v2.19.0...v2.20.0) (2026-06-10)


### Features

* **llm:** add OpenAI provider support ([#56](https://github.com/ionut-t/bark/issues/56)) ([c7d7305](https://github.com/ionut-t/bark/commit/c7d730549ca2a1192ca07fe43c4dddc46b10fffd))

## [2.19.0](https://github.com/ionut-t/bark/compare/v2.18.2...v2.19.0) (2026-06-06)


### Features

* **tui:** add support for persisting relative line numbers in editor ([#54](https://github.com/ionut-t/bark/issues/54)) ([7c71100](https://github.com/ionut-t/bark/commit/7c7110011208f057528d57c120567bbe9be7c9fc))

## [2.18.2](https://github.com/ionut-t/bark/compare/v2.18.1...v2.18.2) (2026-06-03)


### Bug Fixes

* **git:** add context support and timeouts to git operations ([#51](https://github.com/ionut-t/bark/issues/51)) ([ea5af6f](https://github.com/ionut-t/bark/commit/ea5af6fd5055eb78748089687a774edf81b20c3b))

## [2.18.1](https://github.com/ionut-t/bark/compare/v2.18.0...v2.18.1) (2026-06-03)


### Bug Fixes

* **tui:** prevent UI blocking by offloading I/O to commands ([#49](https://github.com/ionut-t/bark/issues/49)) ([93c1969](https://github.com/ionut-t/bark/commit/93c1969b80c01945a4d8e9ded62188a40a468fda))

## [2.18.0](https://github.com/ionut-t/bark/compare/v2.17.0...v2.18.0) (2026-05-31)


### Features

* **actions:** add interactive command for GitHub Actions setup ([#47](https://github.com/ionut-t/bark/issues/47)) ([cb32eb6](https://github.com/ionut-t/bark/commit/cb32eb62054d7d1c830cd435a88dbf0bbe59d4ae))
* **config:** local bark instruction overrides ([#48](https://github.com/ionut-t/bark/issues/48)) ([cc99927](https://github.com/ionut-t/bark/commit/cc99927621ca98ccf85d313198ba3c5c41b2f6e1))
* **llm-context:** add option to limit diff lines in prompts ([#45](https://github.com/ionut-t/bark/issues/45)) ([2b5295d](https://github.com/ionut-t/bark/commit/2b5295d6e0367f11150e02e6dae151246289e931))

## [2.17.0](https://github.com/ionut-t/bark/compare/v2.16.0...v2.17.0) (2026-05-30)


### Features

* **pr:** allow custom instructions and model override ([#43](https://github.com/ionut-t/bark/issues/43)) ([c55e7fa](https://github.com/ionut-t/bark/commit/c55e7fa376e43a129a1b6460e87cf2421e0e1086))


### Bug Fixes

* **bark-review:** fail action on empty review output ([ba0840b](https://github.com/ionut-t/bark/commit/ba0840b30b59faad3b292502f36045fbcb647a39))

## [2.16.0](https://github.com/ionut-t/bark/compare/v2.15.1...v2.16.0) (2026-05-28)


### Features

* **review:** enhance configuration and add github action ([#38](https://github.com/ionut-t/bark/issues/38)) ([d78d351](https://github.com/ionut-t/bark/commit/d78d35168d076ce5ddbf70afe2e14df24b412cf0))


### Bug Fixes

* improve CI reliability and CLI I/O behaviour ([a7ba73f](https://github.com/ionut-t/bark/commit/a7ba73f56007ac34e309edf359769e616151bbc5))

## [2.15.1](https://github.com/ionut-t/bark/compare/v2.15.0...v2.15.1) (2026-05-05)


### Bug Fixes

* **tui:** initialise pr description view correctly ([607575c](https://github.com/ionut-t/bark/commit/607575cd7c71973628cec0e60c6249eb630172d5))

## [2.15.0](https://github.com/ionut-t/bark/compare/v2.14.0...v2.15.0) (2026-05-03)


### Features

* **pr:** generate description from an existing GitHub PR ([e006943](https://github.com/ionut-t/bark/commit/e006943c9a213b1e590c95db6adfa7b6c74dafd3))
* **tui:** stream git commit output ([558c5c8](https://github.com/ionut-t/bark/commit/558c5c85275c5f8fe611d667b46138576aed1236))

## [2.14.0](https://github.com/ionut-t/bark/compare/v2.13.0...v2.14.0) (2026-04-16)


### Features

* **review:** add ability to review GitHub pull requests ([c84894c](https://github.com/ionut-t/bark/commit/c84894c410a8539cbcd52014a18b7e40770a05ec))

## [2.13.0](https://github.com/ionut-t/bark/compare/v2.12.2...v2.13.0) (2026-04-12)


### Features

* **module:** adopt go module v2 conventions ([659f2a8](https://github.com/ionut-t/bark/commit/659f2a8230e851f8fc8b2c6688573db506e8295a))

## [2.12.2](https://github.com/ionut-t/bark/compare/v2.12.1...v2.12.2) (2026-03-24)


### Bug Fixes

* **tui:** improve commit view interaction and error handling ([39cda3f](https://github.com/ionut-t/bark/commit/39cda3ff15711d56aa38de5a64a0969b166ba1fe))

## [2.12.1](https://github.com/ionut-t/bark/compare/v2.12.0...v2.12.1) (2026-03-23)


### Bug Fixes

* **tui:** improve commit error display ([0b2707f](https://github.com/ionut-t/bark/commit/0b2707fa71d482c12f081bdad1ae5dcd05f5511f))

## [2.12.0](https://github.com/ionut-t/bark/compare/v2.11.2...v2.12.0) (2026-03-23)


### Features

* update dependencies and fix breaking changes ([e2034eb](https://github.com/ionut-t/bark/commit/e2034eb80a9a36c3af20442a226b015004954891))

## [2.11.2](https://github.com/ionut-t/bark/compare/v2.11.1...v2.11.2) (2026-03-16)


### Bug Fixes

* **context:** refine LLM call timeout handling ([a342b61](https://github.com/ionut-t/bark/commit/a342b6113da9a25aa4b59b5af55e5736a6163769))

## [2.11.1](https://github.com/ionut-t/bark/compare/v2.11.0...v2.11.1) (2026-03-07)


### Bug Fixes

* **git:** use fmt.Fprintf instead of WriteString with Sprintf ([3f24a16](https://github.com/ionut-t/bark/commit/3f24a163c924c89763a104180291f7cad928526c))

## [2.11.0](https://github.com/ionut-t/bark/compare/v2.10.1...v2.11.0) (2026-03-07)


### Features

* **cli:** add plain mode for non-interactive commands ([af18363](https://github.com/ionut-t/bark/commit/af18363437a725f41c294b94a89ea8689fc9b2b1))

## [2.10.1](https://github.com/ionut-t/bark/compare/v2.10.0...v2.10.1) (2026-01-03)


### Bug Fixes

* **tui:** use named constants for list dimensions ([5233881](https://github.com/ionut-t/bark/commit/5233881801943b208e730bf37e076be20a1222bc))

## [2.10.0](https://github.com/ionut-t/bark/compare/v2.9.0...v2.10.0) (2026-01-03)


### Features

* **config:** add option to set maximum diff lines ([cb43d58](https://github.com/ionut-t/bark/commit/cb43d5838ba136003678a5e436328dfdaa6cd143))

## [2.9.0](https://github.com/ionut-t/bark/compare/v2.8.1...v2.9.0) (2026-01-03)


### Features

* **reviewers:** add new reviewer persona ([ab7274f](https://github.com/ionut-t/bark/commit/ab7274fa1a6cefa9999b9258b6bbe1be956b1f88))

## [2.8.1](https://github.com/ionut-t/bark/compare/v2.8.0...v2.8.1) (2025-11-16)


### Bug Fixes

* **utils:** improve code fence normalisation for diff markers ([a82eedd](https://github.com/ionut-t/bark/commit/a82eedd220b41f1d4c5a52816c626990f0c74b60))

## [2.8.0](https://github.com/ionut-t/bark/compare/v2.7.0...v2.8.0) (2025-11-16)


### Features

* **tui:** provide distinct loading messages for commit phases ([ddc0bb4](https://github.com/ionut-t/bark/commit/ddc0bb4bbf33e1ea356505cc37a152e6dd351a33))

## [2.7.0](https://github.com/ionut-t/bark/compare/v2.6.2...v2.7.0) (2025-11-15)


### Features

* **prompts:** add Rust and Zig code review guidelines ([9399ff2](https://github.com/ionut-t/bark/commit/9399ff2b50358b3038d4ba86f91ba382d62b2237))

## [2.6.2](https://github.com/ionut-t/bark/compare/v2.6.1...v2.6.2) (2025-10-26)


### Bug Fixes

* **git:** increase maximum diff line count ([ec0467e](https://github.com/ionut-t/bark/commit/ec0467e658381f0d986c5157bde92d00d0da9060))

## [2.6.1](https://github.com/ionut-t/bark/compare/v2.6.0...v2.6.1) (2025-10-22)


### Bug Fixes

* **tui:** make commit operation non-blocking ([9a7bcbb](https://github.com/ionut-t/bark/commit/9a7bcbb211d2ea1a69443bbd71aa2e7f09a9b000))

## [2.6.0](https://github.com/ionut-t/bark/compare/v2.5.1...v2.6.0) (2025-10-22)


### Features

* **tui:** improve editor search and review experience ([1358b39](https://github.com/ionut-t/bark/commit/1358b39ebea3bd41ae6beb339f13d62f9de0da8f))

## [2.5.1](https://github.com/ionut-t/bark/compare/v2.5.0...v2.5.1) (2025-10-21)


### Bug Fixes

* **tui:** correct prompt and response display ([0cb0a8a](https://github.com/ionut-t/bark/commit/0cb0a8a22a92cd39380519a0742cbfd1d770176c))

## [2.5.0](https://github.com/ionut-t/bark/compare/v2.4.0...v2.5.0) (2025-10-21)


### Features

* **review:** add ability to review staged changes ([85525b8](https://github.com/ionut-t/bark/commit/85525b884222cd8106b10408d370a51eb0715ee6))

## [2.4.0](https://github.com/ionut-t/bark/compare/v2.3.1...v2.4.0) (2025-10-19)


### Features

* **editor:** enable saving and quitting in TUI models ([3902eb4](https://github.com/ionut-t/bark/commit/3902eb4b1c6d242599041c8c99248e5a81edf1bf))

## [2.3.1](https://github.com/ionut-t/bark/compare/v2.3.0...v2.3.1) (2025-10-17)


### Bug Fixes

* **tui:** ensure commit message handling for commit task ([8bcf336](https://github.com/ionut-t/bark/commit/8bcf336bed7dd5d2d2a13914c1c9cbe652333219))

## [2.3.0](https://github.com/ionut-t/bark/compare/v2.2.1...v2.3.0) (2025-10-17)


### Features

* **commit:** add hint option for commit message type ([62b9aac](https://github.com/ionut-t/bark/commit/62b9aac001ac5309addc658958d2b53f95c8df78))
* **review-options:** add interactive branch input for review ([0ddd459](https://github.com/ionut-t/bark/commit/0ddd459edd7fe81074ad2cccbea2ceb274645adf))

## [2.2.1](https://github.com/ionut-t/bark/compare/v2.2.0...v2.2.1) (2025-10-16)


### Bug Fixes

* **review:** properly integrate branch review option ([c8a8d92](https://github.com/ionut-t/bark/commit/c8a8d92ece930b3ca0cf696188caba2c36ba0d83))

## [2.2.0](https://github.com/ionut-t/bark/compare/v2.1.0...v2.2.0) (2025-10-15)


### Features

* **git:** add total files changed to branch info ([4aeae92](https://github.com/ionut-t/bark/commit/4aeae92b01b0b798d1fbf7819267f1e221858c2d))
* **tui:** add retry and prompt view to commit message screen ([892d3fe](https://github.com/ionut-t/bark/commit/892d3fee0483071af86bc17c5c2f6fccc91379cf))

## [2.1.0](https://github.com/ionut-t/bark/compare/v2.0.0...v2.1.0) (2025-10-15)


### Features

* **tui:** enable direct editing of generated PR description ([08aa113](https://github.com/ionut-t/bark/commit/08aa113c0f931d3abb4459c5d1beb4ccd407e9f7))

## [2.0.0](https://github.com/ionut-t/bark/compare/v1.1.0...v2.0.0) (2025-10-15)


### ⚠ BREAKING CHANGES

* **cli:** Review flags previously available directly on the root `bark` command (e.g., `--as`, `--commit`, `--instructions`, `--branch`, `--staged`) have been moved to the new `bark review` subcommand. Users must now explicitly use `bark review` for all code review operations. For example, `bark --as "Linus"` is now `bark review --as "Linus"`.

### Features

* **cli:** introduce commit and PR message generation ([6f718b7](https://github.com/ionut-t/bark/commit/6f718b70c58c2f7255146cc5867a534d77b312c0))

## [1.1.0](https://github.com/ionut-t/bark/compare/v1.0.1...v1.1.0) (2025-10-14)


### Features

* **edit:** allow editing commit message instruction ([6aa9199](https://github.com/ionut-t/bark/commit/6aa9199c67f9fc7952f672174178b6793d988bc5))
* **reviewers:** add Go co-creators reviewer prompts ([dfd7def](https://github.com/ionut-t/bark/commit/dfd7def03efe73726be3cb58d622231981d86a6a))
* **tui:** add option to skip instruction selection ([80edb31](https://github.com/ionut-t/bark/commit/80edb31bf6fc582aa8a21703dbe02432878ce4a8))

## [1.0.1](https://github.com/ionut-t/bark/compare/v1.0.0...v1.0.1) (2025-10-09)


### Bug Fixes

* **version:** improve version string generation ([813a2ff](https://github.com/ionut-t/bark/commit/813a2ffacec989e9eb8fc1db0fb308eb2b568ada))

## 1.0.0 (2025-10-09)


### Features

* **cli:** add commands to delete and edit assets ([006675a](https://github.com/ionut-t/bark/commit/006675af4284bb304afba26b76030489779ae13d))
* **cmd:** add command to create custom instructions ([bd3a6da](https://github.com/ionut-t/bark/commit/bd3a6daf249d943af9045a301c067fb462462517))
* **init:** set up core AI code reviewer application ([d21326a](https://github.com/ionut-t/bark/commit/d21326a96cb438e029fe5527e93afb7091fa2cb9))
* **tui:** standardise list controls ([839f9ca](https://github.com/ionut-t/bark/commit/839f9cae2739c4607eeff592c55e377604347ad6))
* **version:** add detailed version command output ([47b24e0](https://github.com/ionut-t/bark/commit/47b24e083555c27a0d2696ff040c14c6693cc13b))

## 1.0.0 (2025-10-09)


### Features

* **cli:** add commands to delete and edit assets ([006675a](https://github.com/ionut-t/bark/commit/006675af4284bb304afba26b76030489779ae13d))
* **cmd:** add command to create custom instructions ([bd3a6da](https://github.com/ionut-t/bark/commit/bd3a6daf249d943af9045a301c067fb462462517))
* **init:** set up core AI code reviewer application ([d21326a](https://github.com/ionut-t/bark/commit/d21326a96cb438e029fe5527e93afb7091fa2cb9))
* **tui:** standardise list controls ([839f9ca](https://github.com/ionut-t/bark/commit/839f9cae2739c4607eeff592c55e377604347ad6))
* **version:** add detailed version command output ([47b24e0](https://github.com/ionut-t/bark/commit/47b24e083555c27a0d2696ff040c14c6693cc13b))
