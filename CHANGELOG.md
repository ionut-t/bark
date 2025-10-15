# Changelog

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
