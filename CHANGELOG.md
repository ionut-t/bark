# Changelog

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
