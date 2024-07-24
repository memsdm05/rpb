const button = document.getElementById("powerbutton");
const longpress = document.getElementById("longpress");
const shortpress = document.getElementById("shortpress");

const powerIcon = button.querySelector(".power-icon");
var state = false;

async function press() {
    await fetch("/press", { method: "POST" });
}

async function release() {
    await fetch("/release", { method: "POST" });
}

async function sendTimed(obj, time) {
    await fetch("/release", { method: "POST" });
    addPressedEffect(obj);
    await fetch("/press\?t=" + time + "&wait", { method: "POST" });
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
register(longpress, (obj) => sendTimed(obj, 8));
register(shortpress, (obj) => sendTimed(obj, 1.5));

setInterval(() => {
    fetch("/status")
        .then((resp) => {
            if (resp.status != 200)
                throw new Error("status is not 500")
            return resp
        })
        .then((resp) => resp.json())
        .then((data) => {
            if (state != data.on) {
                state = data.on;
                powerIcon.classList.toggle("on", state);
            }
        })
        .catch(() => _)
}, 500);