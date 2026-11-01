# Spot "Kalender": Schreibtisch mit Bildschirm, Wochenansicht mit farbigen Terminen je Mitarbeiter,
# Tasse und Pflanze.
import sys, os, math
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import kzlib as K
import kzprops as KR
from kzlib import SKY_L, SKY, BLUE, YELLOW, CORAL, MINT, NIGHT, WHITE

R = math.radians


def build():
    dk = K.empty("Tisch", loc=(0, 0.1, 0))
    yield from KR.desk("Tisch", dk, top=WHITE, legs=NIGHT, w=1.4)
    K.shadow("Schatten_Tisch", 0, 0.1, 0.78, 0.3)
    mo = K.empty("Monitor", loc=(-0.05, 0.2, 0.74))
    w, h = 0.9, 0.56
    d = yield from KR.monitor("Monitor", mo, w=w, h=h)
    yield from KR.week_grid("Woche", d, w - 0.06, h - 0.07)
    mu = K.empty("Tasse", loc=(0.52, -0.05, 0.74))
    K.lathe("Tasse_Koerper", [(0.11, 0.0), (0.11, 0.045), (0.0, 0.04), (0.0, 0.0)], n=40, parent=mu, mat=K.toon(CORAL))
    K.ring_slab("Tasse_Henkel", K.ellipse(0, 0, 0.03, 0.035, 32), K.ellipse(0, 0, 0.018, 0.022, 32), -0.007, 0.007,
                parent=mu, mat=K.toon(CORAL), loc=(0.05, 0, 0.055), rot=(R(90), 0, 0))
    K.slab("Tasse_Kaffee", K.ellipse(0, 0, 0.038, 0.038, 32), 0.098, 0.1, parent=mu, mat=K.flat(K.mix_hex(CORAL, NIGHT, 0.55)),
           line=False)
    yield
    pl = K.empty("Pflanze", loc=(-0.56, 0.02, 0.74), scale=(0.5, 0.5, 0.5))
    yield from KR.plant("Pflanze", pl, pot=YELLOW, height=0.9, leaves=6)
    yield


K.run("kalender", build, (1000, 800), samples=24, frame_fill=(0.86, 0.86), cam=(-90, 10))
