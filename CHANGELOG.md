# v0.6.5

* fix error-text in stdout with good lines afterwards

# v0.6.4

* close uuid prefix before waiting for error

# v0.6.3

* add tests for new options
* improve error handling
* fix multiple issues (see commits)

# v0.6.2

* validate argument length

# v0.6.1

* cleaner log format

# v0.6.0

* rewrite to use middlewares
* add lock support

# v0.5.0

* add timeout support

# v0.4.4

* more architectures (386, arm, arm64)

# v0.4.3

* fix output

# v0.4.2

* fix nil pointer if bash is not availible

# v0.4.1

* add quiet times (a whitelist of times where errors are ignored)

# v0.4.0

* [BREAKING] require ascii word boundary on errorstring start

# v0.3.1

* fix default cron name
* fix logging priority

# v0.3.0

* fix logging facility to cron
* change logname go guard

# v0.2.0

* internal errorlog is new io.writer
* redo cli options
* use xid instead of uuid
* add timing infos
