// 从思源宋体 Black 里只取标题（class="display"）用到的字，生成 display.woff2。
// 改了标题文字就重新跑一次：
//   curl -fsSLO https://github.com/notofonts/noto-cjk/raw/main/Serif/SubsetOTF/SC/NotoSerifSC-Black.otf
//   bun add subset-font && bun site/fonts/subset.mjs NotoSerifSC-Black.otf
// 字体以 SIL Open Font License 1.1 授权，见同目录的 OFL.txt。
import { readFileSync, writeFileSync } from "node:fs";
import subsetFont from "subset-font";

const here = new URL(".", import.meta.url);
const html = readFileSync(new URL("../index.html", here), "utf8");
const text = [...html.matchAll(/class="display"[^>]*>(.*?)<\/h[12]>/g)].map((m) => m[1].replace(/<[^>]+>/g, "")).join("") + "，。、：！？「」 ";
const font = readFileSync(process.argv[2]);
const out = await subsetFont(font, text, { targetFormat: "woff2" });
writeFileSync(new URL("display.woff2", here), out);
console.log(`${new Set(text).size} 个字，${out.length} 字节`);
