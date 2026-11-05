import type { AnnualReport } from "./useAnnualReport";

export interface PosterText {
  owner: string;
  first: string;
  second: string;
  words: string;
  days: string;
  characters: string;
  time: string;
  summary: string;
  storySummary: string;
  coverage: string;
  approximate: string;
  footer: string;
}

export async function createSharePoster(
  report: AnnualReport,
  locale: string,
  text: PosterText,
): Promise<string> {
  await document.fonts?.ready;
  const canvas = document.createElement("canvas");
  canvas.width = 1080;
  canvas.height = 1440;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("Canvas is unavailable");
  const style = getComputedStyle(
    document.querySelector(".annual-report") ?? document.documentElement,
  );
  const color = (token: string) => style.getPropertyValue(token).trim();
  const palette = {
    paper: color("--card"),
    ink: color("--foreground"),
    accent: color("--tag-teal-foreground"),
    wash: color("--tag-teal"),
    muted: color("--muted-foreground"),
    border: color("--border"),
  };
  const sans = 'system-ui, "Noto Sans CJK SC", "Microsoft YaHei", sans-serif';
  const serif = '"Songti SC", "Noto Serif CJK SC", "SimSun", Georgia, serif';
  const compact = (value: number) =>
    new Intl.NumberFormat(locale, {
      notation: "compact",
      maximumFractionDigits: 1,
    }).format(value);
  function draw(
    value: string,
    x: number,
    y: number,
    size: number,
    fill: string,
    family = sans,
    maxWidth = 904,
  ) {
    ctx!.fillStyle = fill;
    let fontSize = size;
    ctx!.font = `${fontSize}px ${family}`;
    while (ctx!.measureText(value).width > maxWidth && fontSize > 14) {
      ctx!.font = `${--fontSize}px ${family}`;
    }
    ctx!.fillText(value, x, y);
  }
  ctx.fillStyle = palette.paper;
  ctx.fillRect(0, 0, 1080, 1440);
  ctx.fillStyle = palette.wash;
  ctx.fillRect(32, 32, 1016, 1376);
  ctx.strokeStyle = palette.border;
  ctx.strokeRect(48, 48, 984, 1344);
  draw("Goink", 88, 120, 34, palette.accent);
  draw(text.owner, 88, 172, 25, palette.muted, sans, 790);
  ctx.save();
  ctx.globalAlpha = 0.09;
  ctx.textAlign = "right";
  draw(String(report.Year), 990, 320, 190, palette.accent, serif);
  ctx.restore();
  draw(text.first, 88, 330, 67, palette.ink, serif);
  draw(text.second, 88, 428, 67, palette.accent, serif);
  draw(compact(report.Writing.WordsAdded), 88, 615, 108, palette.accent);
  draw(text.words, 92, 666, 27, palette.muted);
  const seconds = report.Activity.CreativeSeconds;
  const time = new Intl.NumberFormat(locale, {
    maximumFractionDigits: 1,
  }).format(seconds > 0 && seconds < 3600 ? seconds / 60 : seconds / 3600);
  [
    {
      x: 88,
      value: report.Writing.ActiveDays.toLocaleString(locale),
      label: text.days,
    },
    {
      x: 390,
      value: report.Activity.CharactersCreated.toLocaleString(locale),
      label: text.characters,
    },
    { x: 692, value: time, label: text.time },
  ].forEach((item) => {
    draw(item.value, item.x, 787, 52, palette.ink, sans, 270);
    draw(item.label, item.x, 834, 24, palette.muted, sans, 270);
  });
  draw(text.summary, 88, 910, 23, palette.muted);
  draw(text.storySummary, 88, 952, 23, palette.muted);
  ctx.save();
  ctx.translate(280, 970);
  ctx.strokeStyle = palette.accent;
  ctx.lineWidth = 1.2;
  ctx.globalAlpha = 0.28;
  ctx.beginPath();
  ctx.ellipse(258, 180, 161, 64, 0, 0, Math.PI * 2);
  ctx.stroke();
  ctx.beginPath();
  ctx.ellipse(258, 180, 130, 48, 0, 0, Math.PI * 2);
  ctx.stroke();
  ctx.stroke(
    new Path2D(
      "m113 170 60-84 37 44 59-107 42 75 21-26 70 107M174 86l17 63 19-19 28 53 31-160-8 96 33 55 17-76 21 77M113 170q75 42 145 27t144-18l-71 48-82 27-80-27Z",
    ),
  );
  ctx.restore();
  draw(text.coverage, 88, 1280, 21, palette.muted);
  draw(text.approximate, 88, 1318, 21, palette.muted);
  draw(text.footer, 88, 1370, 22, palette.accent, serif);
  return canvas.toDataURL("image/png");
}
