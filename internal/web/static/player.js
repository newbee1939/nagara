// 動画とながら再生（音声のみ）を切り替える。
// スマホでは画面を閉じると <video> は止まるが <audio> は鳴り続けるため、同じファイルを <audio> で流す。
const player = document.querySelector("[data-player]");
const video = player.querySelector("[data-video]");
const audio = player.querySelector("[data-audio]");
const listen = player.querySelector("[data-listen]");
const speeds = player.querySelectorAll("[data-speed]");

const current = () => (player.classList.contains("is-listening") ? audio : video);

// メタデータ読み込み前は currentTime を設定できないので、読み込みを待つ。
const seek = (media, time) => {
  if (media.readyState > 0) {
    media.currentTime = time;
  } else {
    media.addEventListener("loadedmetadata", () => { media.currentTime = time; }, { once: true });
  }
};

listen.addEventListener("click", () => {
  const from = current();
  const to = from === video ? audio : video;
  const playing = !from.paused;

  from.pause();
  seek(to, from.currentTime);
  to.playbackRate = from.playbackRate;
  player.classList.toggle("is-listening", to === audio);
  listen.setAttribute("aria-pressed", String(to === audio));
  if (playing) to.play();
});

for (const button of speeds) {
  button.addEventListener("click", () => {
    current().playbackRate = Number(button.dataset.speed);
    for (const b of speeds) b.setAttribute("aria-pressed", String(b === button));
  });
}

// ロック画面や通知に表示されるタイトル。
if ("mediaSession" in navigator) {
  navigator.mediaSession.metadata = new MediaMetadata({ title: player.dataset.title, artist: "nagara" });
}
