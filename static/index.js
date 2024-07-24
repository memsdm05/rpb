const button = document.getElementById("powerbutton");
const longpress = document.getElementById("longpress");
const shortpress = document.getElementById("shortpress");

const powerIcon = button.querySelector(".power-icon");
const base = "https://rpb.8ken.biz";
var state = false;

async function press() {
    await fetch(base + "/press", { method: "POST" });
}

async function release() {
    await fetch(base + "/release", { method: "POST" });
}

async function sendTimed(obj, time) {
    await fetch(base + "/release", { method: "POST" });
    addPressedEffect(obj);
    await fetch(base + "/press?=" + time + "&wait", { method: "POST" });
    removePressedEffect(obj);
}

function addPressedEffect(element) {
    element.classList.add("pressed");
}

function removePressedEffect(element) {
    element.classList.remove("pressed");
}

function register(element, start, end=null) {
    element.addEventListener('mousedown', () => {
        addPressedEffect(element);
        start(element);
    });
    element.addEventListener('touchstart', (e) => {
        e.preventDefault();
        addPressedEffect(element);
        start(element);
    });
    
    
    if (end) {
        var wrappedEnd = () => {
            removePressedEffect(element);
            end(element);
        }
        element.addEventListener('mouseleave', wrappedEnd);
        element.addEventListener('mouseup', wrappedEnd);
        element.addEventListener('touchend', wrappedEnd);
    }
}

register(button, press, release)
register(longpress, (obj) => sendTimed(obj, 10));
register(shortpress, (obj) => sendTimed(obj, 1.5));

setInterval(() => {
    fetch(base + "/status")
        .then((resp) => resp.json())
        .then((data) => {
            if (state != data.on) {
                state = data.on;
                powerIcon.classList.toggle("on", state);
            }
        })
}, 500);