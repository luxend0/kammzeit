# Kammzeit Illustrationen, Teil 1: Geometrie-Helfer ueber der gemeinsamen toon_lib.
# Den Look (Palette, Toon-Emission-Shader, Nachtblau-Kontur per Inverted Hull, Cycles-CPU-Render,
# sichtbarer Ablauf) liefert toon_lib.py vom zweiten Blender-Agent; hier liegen nur Bausteine
# fuer Figuren und Requisiten (Prismen, Drehkoerper, Lofts, Kapseln, Text, Schatten-Ellipsen)
# und ein Start-Helfer run(), der die Ausgabepfade setzt.
import bpy, bmesh, math, os, sys
from mathutils import Vector

ASSETS = "/tmp/claude-1000/-home-luxend/392722f8-b013-47c4-8bce-99419030ab5b/scratchpad/kammzeit-assets"
HERE = os.path.dirname(os.path.abspath(__file__))
TOON_DIR = os.environ.get("KZ_TOON_DIR", os.path.join(ASSETS, "blender"))
if os.path.exists(os.path.join(HERE, "toon_lib.py")) and not os.environ.get("KZ_TOON_DIR"):
    TOON_DIR = HERE  # Kopie im Repo (fuer Nachbau ohne scratchpad)
sys.path.insert(0, TOON_DIR)
import toon_lib as T  # noqa: E402

FONT_DIR = "/usr/share/fonts/TTF"
TEST = bool(os.environ.get("KZ_TEST"))  # Testlauf: halbe Aufloesung, nur in ill-test/
SCENE = bpy.context.scene

# ---------------------------------------------------------------- Palette (Website v2)
P = T.PAL
SKY_L, SKY, BLUE = P["himmel_hell"], P["himmel"], P["blau"]
YELLOW, CORAL, MINT = P["gelb"], P["koralle"], P["mint"]
NIGHT, WHITE = P["nachtblau"], P["weiss"]
SKIN = [P["haut_hell"], T.mix(P["haut_hell"], P["haut_mittel"], 0.5), P["haut_mittel"],
        T.mix(P["haut_mittel"], P["haut_dunkel"], 0.5), P["haut_dunkel"]]
log = T.log
mix_hex = T.mix


def toon(color, shade=None, steps=2):
    if shade is None:
        return T.toon(color, steps)
    return T.toon(color, steps, name="toon_%s_%d_s%.2f" % (color, steps, shade), shadow=T.mix(color, NIGHT, shade))


def flat(color):
    return T.flat(color)


# ---------------------------------------------------------------- Objekte

def link(obj, parent=None, line=True):
    SCENE.collection.objects.link(obj)
    if parent is not None:
        obj.parent = parent
    if not line:
        obj["kz_no_outline"] = 1
    return obj


def empty(name, loc=(0, 0, 0), rot=(0, 0, 0), parent=None, scale=None):
    """Leeres Gelenk/Gruppe. rot in Radiant."""
    e = bpy.data.objects.new(name, None)
    e.empty_display_size = 0.05
    e.location = loc
    e.rotation_euler = rot
    if scale is not None:
        e.scale = scale
    return link(e, parent)


def _finish(o, parent, mat, line, loc=None, rot=None):
    if loc is not None:
        o.location = loc
    if rot is not None:
        o.rotation_euler = rot
    link(o, parent, line)
    if mat is not None:
        o.data.materials.append(mat)
    return o


def bevel(obj, width, seg=3, angle=35):
    return T.bevel(obj, width, seg, angle)


def _signed_area(pts):
    return 0.5 * sum(pts[i][0] * pts[(i + 1) % len(pts)][1] - pts[(i + 1) % len(pts)][0] * pts[i][1]
                     for i in range(len(pts)))


def mesh_obj(name, bm, smooth=True):
    me = bpy.data.meshes.new(name)
    bm.to_mesh(me)
    bm.free()
    if smooth:
        me.shade_smooth()
    return bpy.data.objects.new(name, me)


def slab(name, pts, z0, z1, parent=None, bev=0.0, seg=3, mat=None, line=True, loc=None, rot=None):
    """Prisma aus 2D-Umriss (x, y) zwischen z0 und z1."""
    pts = list(pts)
    if _signed_area(pts) < 0:
        pts.reverse()
    bm = bmesh.new()
    vb = [bm.verts.new((x, y, z0)) for x, y in pts]
    vt = [bm.verts.new((x, y, z1)) for x, y in pts]
    n = len(pts)
    bm.faces.new(list(reversed(vb)))
    bm.faces.new(vt)
    for i in range(n):
        j = (i + 1) % n
        bm.faces.new((vb[i], vb[j], vt[j], vt[i]))
    o = mesh_obj(name, bm, smooth=bev > 0)
    if bev > 0:
        bevel(o, bev, seg)
    return _finish(o, parent, mat, line, loc, rot)


def ring_slab(name, outer, inner, z0, z1, parent=None, bev=0.0, seg=3, mat=None, line=True, loc=None, rot=None):
    """Prisma mit Loch: outer und inner gleich viele Punkte."""
    if _signed_area(outer) < 0:
        outer = list(reversed(outer))
    if _signed_area(inner) < 0:
        inner = list(reversed(inner))
    bm = bmesh.new()
    ob = [bm.verts.new((x, y, z0)) for x, y in outer]
    ot = [bm.verts.new((x, y, z1)) for x, y in outer]
    ib = [bm.verts.new((x, y, z0)) for x, y in inner]
    it = [bm.verts.new((x, y, z1)) for x, y in inner]
    n = len(outer)
    for i in range(n):
        j = (i + 1) % n
        bm.faces.new((ot[i], ot[j], it[j], it[i]))
        bm.faces.new((ob[j], ob[i], ib[i], ib[j]))
        bm.faces.new((ob[i], ob[j], ot[j], ot[i]))
        bm.faces.new((ib[j], ib[i], it[i], it[j]))
    o = mesh_obj(name, bm, smooth=bev > 0)
    if bev > 0:
        bevel(o, bev, seg)
    return _finish(o, parent, mat, line, loc, rot)


def ellipse(cx, cy, rx, ry, n=96, a0=0.0):
    return [(cx + rx * math.cos(a0 + 2 * math.pi * i / n), cy + ry * math.sin(a0 + 2 * math.pi * i / n)) for i in range(n)]


def rrect(w, h, r, n=12, cx=0.0, cy=0.0):
    return T.rrect(w, h, r, n, cx, cy)


def _superellipse(rx, ry, ex, n):
    out = []
    for i in range(n):
        t = 2 * math.pi * i / n
        c, s = math.cos(t), math.sin(t)
        out.append((rx * math.copysign(abs(c) ** (2 / ex), c), ry * math.copysign(abs(s) ** (2 / ex), s)))
    return out


def loft_z(name, stations, n=48, parent=None, mat=None, line=True, loc=None, rot=None, caps=True):
    """Huelle entlang lokaler Z-Achse. stations: (z, rx, ry, exp[, dx, dy]); rx/ry = 0 ergibt einen Pol."""
    bm = bmesh.new()
    rings = []
    for st in stations:
        z, rx, ry, ex = st[:4]
        dx = st[4] if len(st) > 4 else 0.0
        dy = st[5] if len(st) > 5 else 0.0
        if rx < 1e-7 or ry < 1e-7:
            rings.append(bm.verts.new((dx, dy, z)))
        else:
            rings.append([bm.verts.new((dx + x, dy + y, z)) for x, y in _superellipse(rx, ry, ex, n)])
    for a, b in zip(rings, rings[1:]):
        if isinstance(a, list) and isinstance(b, list):
            for i in range(n):
                j = (i + 1) % n
                bm.faces.new((a[i], a[j], b[j], b[i]))
        elif isinstance(a, list):
            for i in range(n):
                bm.faces.new((a[i], a[(i + 1) % n], b))
        elif isinstance(b, list):
            for i in range(n):
                bm.faces.new((a, b[(i + 1) % n], b[i]))
    if caps and isinstance(rings[0], list):
        bm.faces.new(list(reversed(rings[0])))
    if caps and isinstance(rings[-1], list):
        bm.faces.new(rings[-1])
    bm.normal_update()
    c = sum((v.co for v in bm.verts), Vector()) / len(bm.verts)
    if sum((f.calc_center_median() - c).dot(f.normal) for f in bm.faces) < 0:
        bmesh.ops.reverse_faces(bm, faces=bm.faces[:])
    o = mesh_obj(name, bm)
    for p in o.data.polygons:
        if p.loop_total > 4:
            p.use_smooth = False
    return _finish(o, parent, mat, line, loc, rot)


def lathe(name, profile, n=48, **kw):
    """Drehkoerper um Z aus (z, r)-Profil (von oben nach unten)."""
    return loft_z(name, [(z, r, r, 2.0) for z, r in profile], n=n, **kw)


def ellipsoid(name, rx, ry, rz, n=40, k=20, **kw):
    prof = [(rz * math.cos(math.pi * i / k), math.sin(math.pi * i / k)) for i in range(k + 1)]
    return loft_z(name, [(z, rx * s, ry * s, 2.0) for z, s in prof], n=n, **kw)


def capsule(name, L, r0, r1, n=32, k=8, **kw):
    """Kapsel entlang -Z: Gelenk oben im Ursprung, Laenge L bis zum unteren Kugelmittelpunkt."""
    prof = []
    for i in range(k + 1):
        a = math.pi / 2 * i / k
        prof.append((r0 * math.cos(a), r0 * math.sin(a)))
    for i in range(k + 1):
        a = math.pi / 2 + math.pi / 2 * i / k
        prof.append((-L + r1 * math.cos(a), r1 * math.sin(a)))
    return lathe(name, prof, n=n, **kw)


def text(name, body, size, font="Roboto-Bold.ttf", align="LEFT", parent=None, loc=(0, 0, 0), rot=(0, 0, 0),
         mat=None, spacing=1.0, line=False, extrude=0.0, to_mesh=True):
    """Text, als Mesh umgewandelt (damit Kamera-Rahmung und Konturregeln greifen)."""
    cu = bpy.data.curves.new(name, "FONT")
    cu.body = body
    cu.font = bpy.data.fonts.load(os.path.join(FONT_DIR, font), check_existing=True)
    cu.size = size
    cu.align_x = align
    cu.align_y = "BOTTOM_BASELINE"
    cu.space_character = spacing
    cu.fill_mode = "BOTH"
    cu.extrude = extrude
    tmp = bpy.data.objects.new(name + "_kurve", cu)
    if not to_mesh:
        return _finish(tmp, parent, mat, line, loc, rot)
    SCENE.collection.objects.link(tmp)
    dg = bpy.context.evaluated_depsgraph_get()
    me = bpy.data.meshes.new_from_object(tmp.evaluated_get(dg))
    bpy.data.objects.remove(tmp, do_unlink=True)
    me.name = name
    o = bpy.data.objects.new(name, me)
    return _finish(o, parent, mat, line, loc, rot)


def shadow(name, x, y, rx, ry, alpha=0.12, parent=None, rot=0.0):
    """Flache Schatten-Ellipse in Nachtblau unter Figuren/Objekten (Name beginnt mit 'Schatten')."""
    nm = name if name.startswith("Schatten") else "Schatten_" + name
    return slab(nm, ellipse(0, 0, rx, ry, 64), 0.0, 0.0004, parent=parent, mat=T.shadow_mat(alpha), line=False,
                loc=(x, y, 0.0004), rot=(0, 0, rot))


def bez2(p0, p1, p2, n):
    out = []
    for i in range(n + 1):
        t = i / n
        a, b, c = (1 - t) ** 2, 2 * (1 - t) * t, t * t
        out.append((a * p0[0] + b * p1[0] + c * p2[0], a * p0[1] + b * p1[1] + c * p2[1]))
    return out


def band(center, w0, w1, round_end=True, round_start=False):
    """Umriss eines Bandes entlang einer Mittellinie, Breite w0 -> w1."""
    n = len(center)
    left, right = [], []
    for i, (x, y) in enumerate(center):
        a = center[max(i - 1, 0)]
        b = center[min(i + 1, n - 1)]
        dx, dy = b[0] - a[0], b[1] - a[1]
        l = math.hypot(dx, dy) or 1
        nx, ny = -dy / l, dx / l
        w = (w0 + (w1 - w0) * i / (n - 1)) / 2
        left.append((x + nx * w, y + ny * w))
        right.append((x - nx * w, y - ny * w))

    def cap(p, q, w):
        ang = math.atan2(p[1] - q[1], p[0] - q[0])
        return [(p[0] + w * math.cos(ang + math.pi / 2 - math.pi * k / 12), p[1] + w * math.sin(ang + math.pi / 2 - math.pi * k / 12))
                for k in range(1, 12)]

    pts = left[:]
    if round_end:
        pts += cap(center[-1], center[-2], w1 / 2)
    pts += right[::-1]
    if round_start:
        pts += cap(center[0], center[1], w0 / 2)
    return pts


def descendants(o):
    return T.descendants(o)


def drop_to_floor(root):
    T.drop_to_floor(root)


# ---------------------------------------------------------------- Start

def run(name, build, res, **kw):
    """Startet toon_lib.Run mit den Kammzeit-Ausgabepfaden (KZ_TEST = halbe Aufloesung, Testordner)."""
    if TEST:
        out = os.path.join(ASSETS, "ill-test", name + ".png")
        blend = None
        res = (res[0] // 2, res[1] // 2)
        kw["outline_px"] = kw.get("outline_px", 2.4) / 2
    else:
        out = os.path.join(ASSETS, "ill", "png", name + ".png")
        blend = os.path.join(HERE, name + ".blend")
    kw.setdefault("outline_px", 2.4)
    T.Run(name, build, out, res=res, blend=blend, **kw).start()


# ---------------------------------------------------------------- Kammi

KAMMI_BLEND = os.path.join(ASSETS, "blender", "kammi.blend")


def kammi(name, loc=(0, 0, 0), rot_z=0.0, pose=None, scale=1.0):
    """Maskottchen Kammi aus kammi.blend anhaengen (toon_lib.append_kammi, falls vorhanden).
    Ohne Datei: grauer Platzhalter gleicher Groesse (~0.62 m), nur fuer Testlaeufe."""
    if os.path.exists(KAMMI_BLEND) and hasattr(T, "append_kammi"):
        return T.append_kammi(loc=loc, rot_z=rot_z, scale=scale, pose=pose or "steht")
    log("kammi.blend fehlt, Platzhalter")
    g = empty(name + "_Platzhalter", loc=loc, rot=(0, 0, math.radians(rot_z)))
    c = T.toon(T.PAL["grau"])
    ellipsoid(name + "_Koerper", 0.2, 0.18, 0.24, parent=g, mat=c, loc=(0, 0, 0.26))
    ellipsoid(name + "_Kopf", 0.16, 0.15, 0.15, parent=g, mat=c, loc=(0, -0.02, 0.5))
    return g


def kammi_hold(root, holder, side="R", offset=(0.0, -0.03, 0.0), rot=(0, 0, 0), scale=1.0):
    """Requisit (Empty mit Kindern) in Kammis Pfote legen; Ausrichtung in Weltkoordinaten (Grad),
    damit z. B. ein Display immer zur Kamera zeigt, egal wie der Arm steht."""
    from mathutils import Matrix, Euler
    parts = T.kammi_parts(root)
    hand = parts.get("Kammi_Hand_" + side.upper())
    if hand is None:  # Platzhalter
        holder.location = root.matrix_world.translation + Vector((0, -0.25, 0.4)) + Vector(offset)
        holder.rotation_euler = [math.radians(a) for a in rot]
        holder.scale = (scale, scale, scale)
        return holder
    bpy.context.view_layer.update()
    pos = hand.matrix_world.translation + Vector(offset)
    holder.parent = hand
    holder.matrix_parent_inverse.identity()
    holder.matrix_world = Matrix.LocRotScale(pos, Euler([math.radians(a) for a in rot]).to_quaternion(),
                                             Vector((scale, scale, scale)))
    bpy.context.view_layer.update()
    return holder
