# Model roster

`AGENTS.md` §12.1 keeps the role summary and the loop; this file is the roster
behind it. The references are exact `provider/model#variant` strings and are
looked up with the models tool rather than guessed — a variant that does not
exist fails silently at dispatch. A role with no variant suffix names the
provider's **default** variant — the bare model ID, because `#default` is not a
variant that exists.

Part of #555.

## Roster

| Role        | Model                                          | Fallback chain                                                                              |
| ----------- | ---------------------------------------------- | ------------------------------------------------------------------------------------------- |
| **Worker**  | `opencode-go/deepseek-v4.1-flash`              | `opencode-go/muse-spark-1.3-contributor#xhigh`, then `opencode-go/longcat-2.5-preview-free` |
| **Riddler** | `opencode-go/longcat-2.5-preview-free`         | `opencode-go/deepseek-v4.1-flash`                                                           |
| **Advisor** | `opencode-go/muse-spark-1.3-contributor#xhigh` | none — the requested `#max` variant does not exist; `xhigh` is its highest                  |
| **Judge**   | `opencode-go/deepseek-v4.1-flash#max`          | `opencode-go/longcat-2.5-preview-free`                                                      |
| **Checker** | `opencode-go/muse-spark-1.3-contributor`       | none                                                                                        |

The Worker names a **chain** of fallbacks; the Riddler names one. A fallback is
used when the model before it times out or errors at dispatch, and a dispatch
that falls back says so in its report.

## Reviewer families

The Judge is deliberately a **different model family** from the Advisor: two
reviewers from one family share one family's blind spots. The pairing is what
matters, not any single model — the two reviewers (Advisor and Judge) must differ
in family. Under this roster the Advisor is Muse Spark and the Judge is DeepSeek.

If a future run shows correlated misses between a reviewer and its family-mate,
re-deal the families rather than defending the table.

## Quota-fallback roster (2026-10-21)

**This is the quota-fallback roster while the OpenAI models are rate-limited.**
It contains no OpenAI dependency. The Worker and the Judge now share DeepSeek:
the executor/judge pairing is the one to watch. The Riddler (LongCat) and the
Checker are not reviewers, so the pairing rule does not bind them. The requested
Advisor `#max` variant does not exist — `#xhigh` is Muse Spark's highest
available variant, and an exact reference is what makes dispatch reliable.

When the OpenAI limits clear, restore the primary roster (Worker/Advisor Muse
Spark `xhigh`, Judge GPT-6.1 `high`) after looking the variants up with the
models tool; the cross-family rule still decides the pairing.
