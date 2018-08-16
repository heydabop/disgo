--
-- PostgreSQL database dump
--

-- Dumped from database version 9.6.5
-- Dumped by pg_dump version 9.6.5

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET client_min_messages = warning;
SET row_security = off;

SET search_path = public, pg_catalog;

ALTER TABLE IF EXISTS ONLY public.vote DROP CONSTRAINT IF EXISTS vote_message_id_fkey;
ALTER TABLE IF EXISTS ONLY public.error_ip DROP CONSTRAINT IF EXISTS error_ip_error_id_fkey;
DROP TRIGGER IF EXISTS message_update ON public.message;
ALTER TABLE IF EXISTS ONLY public.vote DROP CONSTRAINT IF EXISTS vote_pkey;
ALTER TABLE IF EXISTS ONLY public.voice_state DROP CONSTRAINT IF EXISTS voice_state_pkey;
ALTER TABLE IF EXISTS ONLY public.user_presence DROP CONSTRAINT IF EXISTS user_presence_pkey;
ALTER TABLE IF EXISTS ONLY public.user_money DROP CONSTRAINT IF EXISTS user_money_guild_id_user_id_key;
ALTER TABLE IF EXISTS ONLY public.user_karma DROP CONSTRAINT IF EXISTS user_karma_guild_id_user_id_key;
ALTER TABLE IF EXISTS ONLY public.shipment DROP CONSTRAINT IF EXISTS shipment_pkey;
ALTER TABLE IF EXISTS ONLY public.shipment DROP CONSTRAINT IF EXISTS shipment_carrier_tracking_number_author_id_key;
ALTER TABLE IF EXISTS ONLY public.reminder DROP CONSTRAINT IF EXISTS reminder_pkey;
ALTER TABLE IF EXISTS ONLY public.pee_log DROP CONSTRAINT IF EXISTS pee_log_create_date_user_id_key;
ALTER TABLE IF EXISTS ONLY public.own_username DROP CONSTRAINT IF EXISTS own_username_pkey;
ALTER TABLE IF EXISTS ONLY public.message DROP CONSTRAINT IF EXISTS message_pkey;
ALTER TABLE IF EXISTS ONLY public.error DROP CONSTRAINT IF EXISTS error_pkey;
ALTER TABLE IF EXISTS ONLY public.error_ip DROP CONSTRAINT IF EXISTS error_ip_error_id_ip_key;
ALTER TABLE IF EXISTS ONLY public.discord_quote DROP CONSTRAINT IF EXISTS discord_quote_pkey;
ALTER TABLE IF EXISTS public.vote ALTER COLUMN id DROP DEFAULT;
ALTER TABLE IF EXISTS public.voice_state ALTER COLUMN id DROP DEFAULT;
ALTER TABLE IF EXISTS public.user_presence ALTER COLUMN id DROP DEFAULT;
ALTER TABLE IF EXISTS public.shipment ALTER COLUMN id DROP DEFAULT;
ALTER TABLE IF EXISTS public.reminder ALTER COLUMN id DROP DEFAULT;
ALTER TABLE IF EXISTS public.own_username ALTER COLUMN id DROP DEFAULT;
ALTER TABLE IF EXISTS public.discord_quote ALTER COLUMN id DROP DEFAULT;
DROP SEQUENCE IF EXISTS public.vote_id_seq;
DROP TABLE IF EXISTS public.vote;
DROP SEQUENCE IF EXISTS public.voice_state_id_seq;
DROP TABLE IF EXISTS public.voice_state;
DROP SEQUENCE IF EXISTS public.user_presence_id_seq;
DROP TABLE IF EXISTS public.user_presence;
DROP TABLE IF EXISTS public.user_money;
DROP TABLE IF EXISTS public.user_karma;
DROP SEQUENCE IF EXISTS public.shipment_id_seq;
DROP TABLE IF EXISTS public.shipment;
DROP SEQUENCE IF EXISTS public.reminder_id_seq;
DROP TABLE IF EXISTS public.reminder;
DROP TABLE IF EXISTS public.pee_log;
DROP SEQUENCE IF EXISTS public.own_username_id_seq;
DROP TABLE IF EXISTS public.own_username;
DROP TABLE IF EXISTS public.message;
DROP TABLE IF EXISTS public.error_ip;
DROP TABLE IF EXISTS public.error;
DROP SEQUENCE IF EXISTS public.discord_quote_id_seq;
DROP TABLE IF EXISTS public.discord_quote;
DROP FUNCTION IF EXISTS public.on_record_update();
DROP EXTENSION IF EXISTS pgcrypto;
DROP EXTENSION IF EXISTS plpgsql;
DROP SCHEMA IF EXISTS public;
--
-- Name: public; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA public;


--
-- Name: SCHEMA public; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON SCHEMA public IS 'standard public schema';


--
-- Name: plpgsql; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS plpgsql WITH SCHEMA pg_catalog;


--
-- Name: EXTENSION plpgsql; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION plpgsql IS 'PL/pgSQL procedural language';


--
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


--
-- Name: EXTENSION pgcrypto; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pgcrypto IS 'cryptographic functions';


SET search_path = public, pg_catalog;

--
-- Name: on_record_update(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION on_record_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ begin new.update_date := now(); return new; end; $$;


SET default_tablespace = '';

SET default_with_oids = false;

--
-- Name: discord_quote; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE discord_quote (
    id integer NOT NULL,
    chan_id character varying(30) NOT NULL,
    author_id character varying(30),
    content text,
    score integer NOT NULL,
    is_fresh boolean NOT NULL
);


--
-- Name: discord_quote_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE discord_quote_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: discord_quote_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE discord_quote_id_seq OWNED BY discord_quote.id;


--
-- Name: error; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE error (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    command text NOT NULL,
    args text,
    error text,
    reported_count integer DEFAULT 0 NOT NULL
);


--
-- Name: error_ip; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE error_ip (
    error_id uuid NOT NULL,
    ip inet NOT NULL
);


--
-- Name: message; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE message (
    id numeric NOT NULL,
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    chan_id numeric NOT NULL,
    author_id numeric NOT NULL,
    content text,
    update_date timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: own_username; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE own_username (
    id integer NOT NULL,
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    author_id character varying(30) NOT NULL,
    username character varying(32) NOT NULL,
    locked_minutes integer NOT NULL,
    guild_id character varying(30)
);


--
-- Name: own_username_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE own_username_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: own_username_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE own_username_id_seq OWNED BY own_username.id;


--
-- Name: pee_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE pee_log (
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    user_id character varying(30)
);


--
-- Name: reminder; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE reminder (
    id integer NOT NULL,
    chan_id character varying(30) NOT NULL,
    author_id character varying(30) NOT NULL,
    send_time timestamp with time zone NOT NULL,
    content text
);


--
-- Name: reminder_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE reminder_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: reminder_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE reminder_id_seq OWNED BY reminder.id;


--
-- Name: shipment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE shipment (
    id integer NOT NULL,
    carrier text NOT NULL,
    tracking_number text NOT NULL,
    chan_id character varying(30) NOT NULL,
    author_id character varying(30) NOT NULL
);


--
-- Name: shipment_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE shipment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: shipment_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE shipment_id_seq OWNED BY shipment.id;


--
-- Name: user_karma; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE user_karma (
    guild_id character varying(30) NOT NULL,
    user_id character varying(30) NOT NULL,
    karma integer NOT NULL
);


--
-- Name: user_money; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE user_money (
    guild_id character varying(30) NOT NULL,
    user_id character varying(30) NOT NULL,
    money double precision NOT NULL
);


--
-- Name: user_presence; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE user_presence (
    id integer NOT NULL,
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    guild_id numeric NOT NULL,
    user_id numeric NOT NULL,
    presence character varying(20) NOT NULL,
    game text
);


--
-- Name: user_presence_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE user_presence_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_presence_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE user_presence_id_seq OWNED BY user_presence.id;


--
-- Name: voice_state; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE voice_state (
    id integer NOT NULL,
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    guild_id character varying(30) NOT NULL,
    chan_id character varying(30),
    user_id character varying(30) NOT NULL,
    session_id character varying(60) NOT NULL,
    deaf boolean NOT NULL,
    mute boolean NOT NULL,
    self_deaf boolean NOT NULL,
    self_mute boolean NOT NULL,
    suppress boolean NOT NULL
);


--
-- Name: voice_state_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE voice_state_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: voice_state_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE voice_state_id_seq OWNED BY voice_state.id;


--
-- Name: vote; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE vote (
    id integer NOT NULL,
    create_date timestamp with time zone DEFAULT now() NOT NULL,
    guild_id character varying(30) NOT NULL,
    message_id bigint NOT NULL,
    voter_id character varying(30) NOT NULL,
    votee_id character varying(30) NOT NULL,
    is_upvote boolean NOT NULL
);


--
-- Name: vote_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE vote_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: vote_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE vote_id_seq OWNED BY vote.id;


--
-- Name: discord_quote id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY discord_quote ALTER COLUMN id SET DEFAULT nextval('discord_quote_id_seq'::regclass);


--
-- Name: own_username id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY own_username ALTER COLUMN id SET DEFAULT nextval('own_username_id_seq'::regclass);


--
-- Name: reminder id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY reminder ALTER COLUMN id SET DEFAULT nextval('reminder_id_seq'::regclass);


--
-- Name: shipment id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY shipment ALTER COLUMN id SET DEFAULT nextval('shipment_id_seq'::regclass);


--
-- Name: user_presence id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY user_presence ALTER COLUMN id SET DEFAULT nextval('user_presence_id_seq'::regclass);


--
-- Name: voice_state id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY voice_state ALTER COLUMN id SET DEFAULT nextval('voice_state_id_seq'::regclass);


--
-- Name: vote id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY vote ALTER COLUMN id SET DEFAULT nextval('vote_id_seq'::regclass);


--
-- Name: discord_quote discord_quote_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY discord_quote
    ADD CONSTRAINT discord_quote_pkey PRIMARY KEY (id);


--
-- Name: error_ip error_ip_error_id_ip_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY error_ip
    ADD CONSTRAINT error_ip_error_id_ip_key UNIQUE (error_id, ip);


--
-- Name: error error_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY error
    ADD CONSTRAINT error_pkey PRIMARY KEY (id);


--
-- Name: message message_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY message
    ADD CONSTRAINT message_pkey PRIMARY KEY (id);


--
-- Name: own_username own_username_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY own_username
    ADD CONSTRAINT own_username_pkey PRIMARY KEY (id);


--
-- Name: pee_log pee_log_create_date_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY pee_log
    ADD CONSTRAINT pee_log_create_date_user_id_key UNIQUE (create_date, user_id);


--
-- Name: reminder reminder_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY reminder
    ADD CONSTRAINT reminder_pkey PRIMARY KEY (id);


--
-- Name: shipment shipment_carrier_tracking_number_author_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY shipment
    ADD CONSTRAINT shipment_carrier_tracking_number_author_id_key UNIQUE (carrier, tracking_number, author_id);


--
-- Name: shipment shipment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY shipment
    ADD CONSTRAINT shipment_pkey PRIMARY KEY (id);


--
-- Name: user_karma user_karma_guild_id_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY user_karma
    ADD CONSTRAINT user_karma_guild_id_user_id_key UNIQUE (guild_id, user_id);


--
-- Name: user_money user_money_guild_id_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY user_money
    ADD CONSTRAINT user_money_guild_id_user_id_key UNIQUE (guild_id, user_id);


--
-- Name: user_presence user_presence_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY user_presence
    ADD CONSTRAINT user_presence_pkey PRIMARY KEY (id);


--
-- Name: voice_state voice_state_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY voice_state
    ADD CONSTRAINT voice_state_pkey PRIMARY KEY (id);


--
-- Name: vote vote_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY vote
    ADD CONSTRAINT vote_pkey PRIMARY KEY (id);


--
-- Name: message message_update; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER message_update BEFORE UPDATE ON message FOR EACH ROW EXECUTE PROCEDURE on_record_update();


--
-- Name: error_ip error_ip_error_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY error_ip
    ADD CONSTRAINT error_ip_error_id_fkey FOREIGN KEY (error_id) REFERENCES error(id);


--
-- Name: vote vote_message_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY vote
    ADD CONSTRAINT vote_message_id_fkey FOREIGN KEY (message_id) REFERENCES message(id);


--
-- PostgreSQL database dump complete
--

