- List queries no longer need a default account. With no filter argument,
  `akt query deployment`, `market order`, `market bid`, `market lease` and
  `cert list` list every matching resource on the network, one page at a time
  (`--limit`, default 100), in either `--by` perspective. A bare state keyword
  narrows that list, so `akt query deployment closed` returns closed
  deployments network-wide instead of failing with "no default account set".
  The default account now fills in only the numeric dseq shorthand. Help text
  and docs point unfiltered lists at `--page-key` for the next page, since the
  chain accepts `--page` and `--offset` only together with a state filter.
- `akt query deployment group` gains a list mode: with no filter, or an owner
  address only, it pages through deployments and prints their groups, since
  the chain has no query that lists groups. Its structured output is an object
  with `groups` and the deployment page's `pagination`, and it refuses
  `--page` and `--offset`, which need a state filter it cannot take.
