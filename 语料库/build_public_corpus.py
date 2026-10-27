# -*- coding: utf-8 -*-
"""生成可公开发布的 52 档仿真默认语料 corpus-public-v1。"""
import gzip
import importlib.util
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent
OUTPUT = ROOT.parent / "internal" / "store" / "seed" / "corpus.json.gz"

spec = importlib.util.spec_from_file_location("gen_corpus", ROOT / "gen_corpus.py")
gen_corpus = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gen_corpus)

TOKEN_LEVELS = [
    "1K", "2K", "2.5K", "3K", "4K", "5K", "6K", "7K", "8K", "9K", "10K",
    "15K", "20K", "25K", "30K", "32K", "35K", "40K", "50K", "60K", "64K",
    "70K", "80K", "90K", "100K", "120K", "128K", "140K", "160K", "180K",
    "200K", "220K", "240K", "256K", "280K", "300K", "340K", "380K", "420K",
    "480K", "512K", "550K", "600K", "650K", "700K", "750K", "800K", "850K",
    "900K", "950K", "1000K", "1024K",
]


def token_count(tag: str) -> int:
    value = float(tag[:-1])
    return int(value * 1000)


def target_chars(tag: str) -> int:
    # 与项目当前中文语料口径一致：约 1.5 字符/token。
    return int(round(token_count(tag) * 1.5))


def main() -> None:
    texts = {}
    for tag in sorted(TOKEN_LEVELS, key=token_count):
        texts[tag] = gen_corpus.build(tag, target_chars(tag))

    payload = {
        "version_no": "corpus-public-v1",
        "note": "公开默认语料(基于 gen_corpus.py 的无版权风险业务仿真文本，52 档)",
        "texts": texts,
    }
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    with OUTPUT.open("wb") as fh:
        with gzip.GzipFile(fileobj=fh, mode="wb", mtime=0) as gz:
            gz.write(json.dumps(payload, ensure_ascii=False).encode("utf-8"))
    print(f"generated {OUTPUT} with {len(texts)} levels")


if __name__ == "__main__":
    main()

