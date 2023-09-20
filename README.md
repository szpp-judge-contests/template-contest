# {{ .ContestName }}

- URL: {{ .ContestURL }}

## 問題の作り方

### 1. `task.yaml`

```yaml
title: a + b  # 問題名
writer: szppi  # 作問者の szpp-judge id
checker: normal  # checker の種類(TODO: doc)
verifier: verifier.cpp  # テストケース入力検証機のファイル名(基本 verifier.cpp)
correct: correct.cpp  # writer 解のファイル名(基本 correct.cpp)
time_limit: 2000  # 実行時間制限[ms]
memory_limit: 1024  # 実行メモリ制限[MB]
difficulty: beginner  # 難易度(TODO: doc)
testcase_sets:  # テストケースセット
  sample:  # sample(入力例として使われる)
    score: 0  # テストケースセットを全て AC した時の得点
    list:  # テストケースセットに属するテストケース一覧
      - testcase_01  
  all:
    score: 100
    list:
      - testcase_01
      - testcase_02
      - testcase_03
      - testcase_04
      - testcase_05
testcases:
  - name: testcase_01  # テストケース(in と out にあるやつ)
    description: $5 + 3 = 8$ です。  # 入力例で表示される
  - name: testcase_02
  - name: testcase_03
  - name: testcase_04
  - name: testcase_05
```

### 2. `statement.md`

「問題文」、「制約」、「入力」、「出力」の4つのセクションを設けて以下のように記述してください。

```markdown
## 問題文

$A + B$ の答えを求めてください。

## 制約

- $1 \leq A, B \leq 10^{18}$

## 入力

入力は以下の形式で標準入力から与えられます。

```
$A$ $B$
```

## 出力

答えを出力してください。
```

### 3. `verifier.cpp`

テストケースの入力形式を検証するソースファイルです。
[ここ](https://scrapbox.io/ecasdqina-cp/testlib_read_%E7%B3%BB%E3%81%BE%E3%81%A8%E3%82%81) を参考にするなどして記述してください。

### 4. `correct.cpp`

writer 解を記述してください。

### 5. testcases

以下のようなディレクトリ構造で入力と出力のテストケースをおいてください。

```shell
testcases
├── in
│  ├── testcase_01.txt
│  ├── testcase_02.txt
│  ├── testcase_03.txt
│  ├── testcase_04.txt
│  └── testcase_05.txt
└── out
   ├── testcase_01.txt
   ├── testcase_02.txt
   ├── testcase_03.txt
   ├── testcase_04.txt
   └── testcase_05.txt
```

## CI/CD

### check_task

task の形式が正しいかどうかを検証します。
ブランチへの push、PR のタイミングで走ります。

### deploy

task を backend サーバにアップロードします。
