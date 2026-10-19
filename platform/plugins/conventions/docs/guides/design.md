# パターンを当てはめる

変更を設計するときと、差分をレビューするときに、当てはめる設計のパターンを選んで使う手順。共通の手順で、目的の手順の
段の中で使う（[目的の手順と共通の手順](../architecture/documentation.md#procedures)）。どの変更にどのパターンを当てはめるかは[変更とパターン](../reference/changes.md#patterns "在りか")、レビューでの指摘の仕方は
[指摘](../reference/reviews.md#findings "在りか")にある。

1. **選ぶ**：変更の種類を見て、[変更とパターン](../reference/changes.md#patterns)の表から当てはまるパターンを選ぶ
   （[確立した考え方に拠る](../../../principles/docs/reference/principles.md#established)）。
2. **確かめる**：各パターンの形とよくある失敗を、[設計のパターン](../../../principles/docs/reference/patterns.md)で確かめる。
3. **当てはめる**：作るときはその形に沿って設計し、レビューでは守られていない点を、パターンの名前と具体的な組み合わせで指摘する。
