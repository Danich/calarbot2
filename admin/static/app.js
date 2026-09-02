// Панель только мутирует состояние: разметку целиком отдаёт сервер, поэтому
// здесь нет ни шаблонов, ни сборки форм.
const api = (method, url, body) =>
  fetch(url, {
    method,
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : null,
  }).then((r) => {
    if (!r.ok) return r.text().then((t) => Promise.reject(new Error(t || r.status)));
  });

const fail = (e) => alert("Не сохранилось: " + e.message);

document.querySelectorAll(".seg").forEach((seg, i) => {
  seg.querySelectorAll("input[type=radio]").forEach((r) => (r.name = "seg-" + i));
});

document.querySelectorAll(".tab").forEach((tab) => {
  tab.onclick = () => {
    document.querySelectorAll(".tab").forEach((t) => t.classList.toggle("is-active", t === tab));
    document.getElementById("channels").hidden = tab.dataset.tab !== "channels";
    document.getElementById("dms").hidden = tab.dataset.tab !== "dms";
  };
});

document.querySelectorAll(".js-expand").forEach((btn) => {
  btn.onclick = () => {
    const panel = btn.closest(".card").querySelector(".panel");
    panel.hidden = !panel.hidden;
    btn.style.transform = panel.hidden ? "" : "rotate(180deg)";
  };
});

document.querySelectorAll(".js-toggle").forEach((radio) => {
  radio.onchange = () => {
    const row = radio.closest(".module");
    const chat = radio.closest(".card").dataset.chat;
    const enabled = radio.value === "1";
    const settings = row.querySelector(".settings");
    // Настройки прячем сразу, но не стираем: значения живут в базе и вернутся
    // такими же, когда модуль включат обратно.
    if (settings) settings.hidden = !enabled;
    api("PATCH", `/api/chats/${chat}/modules/${row.dataset.module}`, { enabled }).catch(fail);
  };
});

document.querySelectorAll(".js-field").forEach((input) => {
  input.onchange = () => {
    const row = input.closest(".module");
    const chat = input.closest(".card").dataset.chat;
    // Тип берём из самого элемента, а не угадываем по значению: у чекбокса
    // пустой value не бывает, а у number и text пустая строка означает разное
    // (число нельзя послать пустым, а null — это «вернуть дефолт»).
    let value;
    if (input.tagName === "SELECT") {
      value = input.value;
    } else if (input.type === "checkbox") {
      value = input.checked;
    } else if (input.type === "number") {
      value = input.value === "" ? null : Number(input.value);
    } else {
      value = input.value === "" ? null : input.value;
    }
    api("PATCH", `/api/chats/${chat}/settings/${row.dataset.module}`, {
      [input.dataset.key]: value,
    }).catch(fail);
  };
});

const dialog = document.getElementById("confirm");
let pending = null;

document.querySelectorAll(".js-leave").forEach((btn) => {
  btn.onclick = () => {
    const card = btn.closest(".card");
    pending = card;
    document.getElementById("confirm-text").textContent =
      "Выйти из «" + card.querySelector(".name").textContent + "»?";
    dialog.showModal();
  };
});

document.getElementById("confirm-no").onclick = () => dialog.close();
document.getElementById("confirm-yes").onclick = () => {
  const card = pending;
  dialog.close();
  api("POST", `/api/chats/${card.dataset.chat}/leave`)
    .then(() => card.remove())
    .catch(fail);
};
