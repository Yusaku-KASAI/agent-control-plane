# The form of a review and of the answers to it: what prctl accepts as JSON, and the Markdown it
# posts. The severities and perspectives are those of the conventions (reviews.md: 指摘, 観点).

def severities: ["必須", "提案", "質問", "細部"];
def perspectives: ["別の形", "依頼との一致", "正しさ", "設計", "セキュリティ", "文書とコメント", "検査"];
def actions: ["直した", "直さない", "Issue にした", "答えた"];

def text: type == "string" and (gsub("\\s"; "") != "");
def oneline: tostring | gsub("\\s*\n\\s*"; " ");
def cell: oneline | gsub("\\|"; "\\|");

# review_errors: what is missing or wrong in a review, one string each; empty when it is right.
def review_errors:
  if type != "object" then ["the review is not a JSON object"] else
    . as $r
    | [ (if (.scope | IN("since", "pr")) then empty else "scope is \"since\" (since the last review) or \"pr\" (the whole PR)" end),
      (if (.perspectives | type) == "array" and (.perspectives | length) > 0
         and all(.perspectives[]; IN(perspectives[]))
       then empty else "perspectives lists those looked at, from: \(perspectives | join(", "))" end),
      (if .scope == "pr" and ((.perspectives // []) | unique | length) != (perspectives | length)
       then "a review of the whole PR looks at every perspective" else empty end),
      ((perspectives - (.perspectives // []) - [(.skipped // [])[].perspective?]) as $left
       | if .scope == "since" and ($left | length) > 0
         then "skipped says why these were not looked at: \($left | join(", "))" else empty end),
      (if (.skipped // []) | all(.[]; (.perspective | IN(perspectives[])) and (.reason | text))
       then empty else "each of skipped has a perspective and a reason" end),
      ([(.skipped // [])[].perspective?] - ([(.skipped // [])[].perspective?] - (.perspectives // []))
       | if length > 0 then "perspectives both looked at and skipped: \(join(", "))" else empty end),
      (if (.findings | type) == "array" then empty else "findings is a list (empty when nothing was found)" end),
      ((.findings // []) | to_entries[] | .key as $i | .value
        | ("finding \($i + 1)") as $f
        | (if (.severity | IN(severities[])) then empty else "\($f): severity is one of \(severities | join(", "))" end),
          (if (.perspective | IN(($r.perspectives // [])[])) then empty else "\($f): perspective is one of those looked at" end),
          (if (.location | text) and (.summary | text) and (.basis | text) and (.problem | text)
             and (.fix | text) and (.verified | text)
           then empty else "\($f): location, summary, basis, problem, fix and verified are all written" end))
    ] end;

# answer_errors($review): what is missing or wrong in answers to $review; every finding is answered.
def answer_errors($review):
  ($review.findings | length) as $n
  | if type != "array" then ["the answers are a JSON list"] else
      [ (.[] | ("answer to \(.finding)") as $a
          | (if (.finding | type) == "number" and .finding == (.finding | floor) and .finding >= 1 and .finding <= $n
             then empty else "\($a): finding is the number of a finding, 1 to \($n)" end),
            (if (.action | IN(actions[])) then empty else "\($a): action is one of \(actions | join(", "))" end),
            (if .action == "直した" and ((.commit | text) | not) then "\($a): a fix names its commit" else empty end),
            (if .action == "Issue にした" and ((.issue | type) != "number") then "\($a): names its issue" else empty end),
            (if (.action | IN("直さない", "答えた")) and ((.reason | text) | not)
             then "\($a): says why, or what the answer is" else empty end)),
        ([range(1; $n + 1)] - [.[].finding] | if length > 0 then "findings not answered: \(map(tostring) | join(", "))" else empty end),
        ([group_by(.finding)[] | select(length > 1) | .[0].finding | tostring]
         | if length > 0 then "findings answered more than once: \(join(", "))" else empty end)
      ] end;

def answer_text:
  if .action == "直した" then "直した（\(.commit)）\(if .reason then "。\(.reason)" else "" end)"
  elif .action == "Issue にした" then "Issue にした（#\(.issue)）\(if .reason then "。\(.reason)" else "" end)"
  else "\(.action)：\(.reason)" end;

# render_review($title; $answers): the body of the PR review; $answers is a list, or null.
def render_review($title; $answers):
  . as $r
  | ($answers // [] | map({key: (.finding | tostring), value: .}) | from_entries) as $by
  | [ "## \($title)",
      "",
      "見た観点：\([perspectives[] | select(IN($r.perspectives[]))] | join("、"))"
        + (if ($r.skipped // []) | length > 0
           then "。外した観点：\($r.skipped | sort_by(.perspective as $p | perspectives | index($p)) | map("\(.perspective)（\(.reason)）") | join("、"))" else "" end),
      (if $r.note then "", $r.note else empty end),
      "",
      (if ($r.findings | length) == 0 then "指摘なし。" else
        "| # | 重さ | 観点 | 場所 | 要約 | 拠り所 |",
        "| --- | --- | --- | --- | --- | --- |",
        ($r.findings | to_entries[]
          | "| \(.key + 1) | \(.value.severity) | \(.value.perspective) | \(.value.location | cell) | \(.value.summary | cell) | \(.value.basis | cell) |"),
        ($r.findings | to_entries[] | .key as $i | .value
          | "", "### \($i + 1). \(.summary | oneline)", "",
            "- **何が問題か**：\(.problem)",
            "- **直し方の案**：\(.fix)",
            "- **確かめ方**：\(.verified)",
            (if .notes then "- **補足**：\(.notes)" else empty end),
            (if $by[($i + 1) | tostring] then "- **答え**：\($by[($i + 1) | tostring] | answer_text)" else empty end))
      end)
    ] | join("\n");

# record: what of a review is kept in its hidden copy: enough to tell its scope and to check the
# answers to it, as the PR review's body has a limit.
def record: {scope, perspectives, findings: [.findings[] | {severity, perspective, summary}],
  answered: [(.answers // [])[].finding]};

# final_errors($cap; $since; $head): whether the PR may end, from the facts of its reviews in the
# head's history, oldest first: [{commit, at, minor, scope, findings, answered}] (at: the commit's
# place in the history; $head: the head's). What stands in the way, as [{kind, text}], kind one of
# none (no review of the whole PR), unrecorded (a push not recorded yet), changed, must, open (not
# answered) and back (hand it back); empty when it may end. The reviews of the whole PR are counted
# against $cap from the place $since on (the PR last went back there); back comes once the count
# reaches $cap and the PR still needs another such review.
def final_errors($cap; $since; $head):
  . as $rs
  | ([$rs[] | select(.scope == "pr" and .at > $since)] | length) as $wholes
  | ([$rs[] | select(.scope == "pr")] | last) as $r
  | if $r == null then
      [{kind: "none", text: "no review of the whole PR (scope \"pr\", every perspective) yet"}]
    else
      # Changes after it: records of later commits other than fixes of nitpicks. Another review of the
      # same commit is no change.
      ([$rs[] | select(.at > $r.at and (.minor | not))] | length) as $after
      # A head past the last record is a push not recorded yet: whether a change or a fix of
      # nitpicks is for its record to say.
      | ($head > ([$rs[].at] | max)) as $unrecorded
      | ([$r.findings | to_entries[] | select(.value.severity == "必須") | .key + 1]) as $musts
      | ([range(1; ($r.findings | length) + 1)] - $r.answered) as $open
      | [ (if $unrecorded then {kind: "unrecorded", text: "the push after the last record is not recorded yet: record it first (prctl unreviewed tells how)"} else empty end),
          (if $after > 0 then {kind: "changed", text: "changes other than fixes of nitpicks since the review of the whole PR at \($r.commit[0:12]); review the whole PR again"} else empty end),
          (if $after == 0 and ($musts | length) > 0 then {kind: "must", text: "the review of the whole PR at \($r.commit[0:12]) has findings that must be fixed: \($musts | map(tostring) | join(", ")); fix them, then review the whole PR again"} else empty end),
          (if $after == 0 and ($open | length) > 0 then {kind: "open", text: "findings of the review of the whole PR at \($r.commit[0:12]) not answered: \($open | map(tostring) | join(", ")); answer them (prctl answer \($r.commit[0:12]) <answers-json>)"} else empty end) ]
      | if length > 0 and $wholes >= $cap and any(.kind == "changed" or .kind == "must")
        then [{kind: "back", text: "the whole PR was reviewed \($wholes) times since it last went back and still does not settle: stop, and hand it back to the requester with what remains (\(map(.text) | join("; ")))"}]
        else . end
    end;

# render_answers($title): the comment that answers a review.
def render_answers($title):
  [ "## \($title)", "",
    "| # | 答え |", "| --- | --- |",
    (sort_by(.finding)[] | "| \(.finding) | \(answer_text | cell) |")
  ] | join("\n");
