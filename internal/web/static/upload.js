// 選んだ動画の再生時間をブラウザで読み取り、フォームに添える。
// サーバー側で動画を解析しなくて済む（読めなければ空のまま送り、「不明」として扱われる）。
const form = document.querySelector("[data-upload]");
const media = form.querySelector("[data-media]");
const duration = form.querySelector("[data-duration]");
const submit = form.querySelector("[data-submit]");

media.addEventListener("change", () => {
  duration.value = "";
  const file = media.files[0];
  if (!file) return;

  const probe = document.createElement("video");
  probe.preload = "metadata";
  probe.addEventListener("loadedmetadata", () => {
    if (Number.isFinite(probe.duration)) duration.value = probe.duration;
    URL.revokeObjectURL(probe.src);
  }, { once: true });
  probe.src = URL.createObjectURL(file);
});

// 二重送信を防ぐ。
form.addEventListener("submit", () => {
  submit.disabled = true;
  submit.textContent = "アップロード中…";
});

// 戻るボタンでこの画面に戻ったとき（bfcache）に、ボタンを押せる状態に戻す。
window.addEventListener("pageshow", () => {
  submit.disabled = false;
  submit.textContent = "公開する";
});
