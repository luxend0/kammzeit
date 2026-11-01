# Spot "Kundenkartei": Karteikasten mit farbigen Reitern, Kundenkarte mit Farbrezeptur und Terminhistorie,
# Farbschale mit Pinsel, Bleistift.
import sys, os, math
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import kzlib as K
import kzprops as KR
from kzlib import SKY_L, SKY, BLUE, YELLOW, CORAL, MINT, NIGHT, WHITE

R = math.radians


def build():
    bx = K.empty("Kasten", loc=(-0.12, 0.12, 0), rot=(0, 0, R(-8)))
    yield from KR.card_box("Kasten", bx, color=BLUE)
    K.shadow("Schatten_Kasten", -0.12, 0.12, 0.36, 0.2)
    cc = K.empty("Karte", loc=(0.12, -0.16, 0), rot=(0, 0, R(6)))
    yield from KR.customer_card("Karte", cc)
    K.shadow("Schatten_Karte", 0.12, -0.16, 0.26, 0.06)
    tb = K.empty("Schale", loc=(0.58, 0.02, 0))
    yield from KR.tint_bowl("Schale", tb, color=WHITE, paint=CORAL)
    K.shadow("Schatten_Schale", 0.58, 0.02, 0.11, 0.05)
    pe = K.empty("Stift", loc=(-0.5, -0.3, 0), rot=(0, 0, R(18)))
    yield from KR.pencil("Stift", pe, color=YELLOW)
    yield


K.run("kartei", build, (1000, 800), samples=24, frame_fill=(0.9, 0.9), cam=(-90, 24))
